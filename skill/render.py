#!/usr/bin/env python3
"""Render a feature panel (standalone HTML) from a feature.json file.

Usage: python3 render.py path/to/feature.json [--out panel.html] [--open]
       python3 render.py --index [features-dir] [--open]   (all panels + portfolio page)
Validates the data, derives statuses, and fails with a clear message if the
file breaks the rules in SCHEMA.md. Python 3 standard library only.
"""
import re, json, sys, os, datetime, webbrowser
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common

HERE = os.path.dirname(os.path.abspath(__file__))
FACT = ('y', 'fixed', 'bad', 'p', 'q', 'w')   # done, problem found and fixed, open problem, partial, not done, in work now
DONE = ('y', 'fixed', 'n')                # kinds that count as finished
SEV = ('high', 'mid', 'low')
WHO = ('you', 'look', 'agent', 'later')   # who has to act: you / needs your look / the agent / a later phase


class DataError(Exception):
    pass


def fail(msg):
    raise DataError(msg)


def stage_state(facts):
    """Colour of a stage says how worried to be, not whether anything is left:
    no = nothing done, bad = an open problem, ok = all done, almost = 80% or more done, part = the rest."""
    kinds = ['y' if f[0] in DONE else f[0] for f in facts]
    if all(k == 'q' for k in kinds):          # also a planned stage with no facts yet
        return 'no'
    if 'bad' in kinds:
        return 'bad'
    done = kinds.count('y')
    if done == len(kinds):
        return 'ok'
    return 'almost' if done / len(kinds) >= 0.8 else 'part'


# What a stage is about. A feature decides for itself which of these it has: they are marks on the stages
# of its plan, not steps every feature must pass. "kind" in feature.json wins; without it the title is read.
KINDS = {'plan': ['📝', 'Planning'], 'research': ['🔍', 'Research'], 'code': ['🛠', 'Writing code'], 'test': ['🧪', 'Testing'],
         'review': ['👀', 'Review'], 'deploy': ['🚀', 'Deployment'], 'docs': ['📄', 'Documentation']}
HEAD = re.compile(r'^(?:фаза|этап|phase|stage|step)\s*\d+[^.:]*[.:]\s*')
GUESS = (('deploy', r'deploy|release|rollout|выкат|выклад|релиз|деплой|подготовка к выкладке'),
         ('review', r'(?:code[ -]?)?review|(?:код[ -]?)?ревью|аудит|audit'),
         ('test', r'tests?\b|testing|тест|проверк|автопроверк|ручная проверка|сетка безопасности'),
         ('docs', r'docs?\b|documentation|документ'),
         ('research', r'research|analysis|investigat|анализ|исследован|разведка|разобраться|подготовка и инвентар'),
         ('plan', r'plan|план|согласовать|спроектировать|проектирование'))


def stage_kind(s):
    k = s.get('kind')
    if k is not None:
        if k not in KINDS:
            fail('stage %s: kind must be one of %s' % (s['n'], ', '.join(KINDS)))
        return k
    # only what the title starts with counts: a word somewhere inside it ("..., tests") says too little
    title = HEAD.sub('', str(s.get('title', '')).lower().strip())
    for kind, pattern in GUESS:
        if re.match(pattern, title):
            return kind
    return ''


def progress(stages):
    """Percent of the plan that is done. Every stage weighs the same: a stage counts by the share of its
    items that are done (partly done = half, "not needed" is left out), a stage without items counts as 0."""
    if not stages:
        return 0
    total = 0.0
    for s in stages:
        kinds = [f[0] for f in s['facts'] if f[0] != 'n']
        if kinds:
            total += sum(1 if k in DONE else .5 if k == 'p' else 0 for k in kinds) / len(kinds)
        elif s['facts']:
            total += 1                                # everything in it was marked as not needed
    return round(100 * total / len(stages))


def current_stage(stages):
    """Number of the stage the work is at: the last stage with an item marked "in work" (w),
    else the furthest stage that was started, else the first one."""
    for want in (lambda s: any(f[0] == 'w' for f in s['facts']), lambda s: s['state'] != 'no'):
        hit = [s['n'] for s in stages if want(s)]
        if hit:
            return hit[-1]
    return stages[0]['n'] if stages else None


HISTORY_KEEP = 300


def track(src, data):
    """Keep history.json next to feature.json: what changed between two versions of the file, and when.
    Written here, never by Claude. Items are matched by id (stage number + text); an item whose text was
    rewritten in place (same stage, same position) is treated as the same item. The first run only takes
    a snapshot, so an existing feature does not show up as "everything is new".
    Returns (changes, time of the last update in ms)."""
    path = os.path.join(os.path.dirname(src), 'history.json')
    try:
        mtime = int(os.stat(src).st_mtime * 1000)
    except OSError:
        return [], 0
    h = common.read_json(path)
    if not isinstance(h, dict):
        h = {}
    changes = [c for c in h.get('changes', []) if isinstance(c, dict)]
    snap = {str(s['n']): [[fid, f[0]] for fid, f in zip(s['fids'], s['facts'])] for s in data['stages']}
    old = h.get('snap')
    if old == snap:
        return changes, mtime
    try:                                              # a "not needed" decision changes overrides.json only
        when = max(mtime, int(os.stat(os.path.join(os.path.dirname(src), 'overrides.json')).st_mtime * 1000))
    except OSError:
        when = mtime
    if isinstance(old, dict):
        for s in data['stages']:
            before = [x for x in old.get(str(s['n']), []) if isinstance(x, list) and len(x) == 2]
            was = dict(before)
            now_ids = set(s['fids'])
            for i, (fid, f) in enumerate(zip(s['fids'], s['facts'])):
                prev = was.get(fid)
                if prev is None and i < len(before) and before[i][0] not in now_ids:
                    gone, prev = before[i]            # same place, new text: the same item, rewritten
                    for c in changes:
                        if c.get('fid') == gone:
                            c['fid'] = fid
                if prev != f[0]:
                    changes.append({'ts': when, 'fid': fid, 'stage': s['n'], 'was': prev, 'now': f[0], 't': f[1]})
    changes = changes[-HISTORY_KEEP:]
    try:
        common.atomic_write_json(path, {'snap': snap, 'changes': changes})
    except OSError:
        pass                                          # a read-only folder: the page still renders
    return changes, mtime


def prepare(d, overrides=None):
    for key in ('title', 'projects', 'stages'):
        if key not in d:
            fail('missing "%s"' % key)
    projects = d['projects']
    if not projects or not all(isinstance(p, str) for p in projects):
        fail('"projects" must be a non-empty list of names')
    d.setdefault('sub', '')
    d.setdefault('summary', '')
    for k in ('decisions', 'questions', 'blockers', 'nextSteps'):
        d.setdefault(k, [])
    # stages: ordered, facts validated, state derived (never typed by hand)
    last = 0
    for s in d['stages']:
        if not isinstance(s.get('n'), int) or s['n'] <= last:
            fail('stage numbers must be integers in increasing order (got %r)' % s.get('n'))
        last = s['n']
        for key in ('title', 'verdict', 'plan', 'facts'):
            if key not in s:
                fail('stage %s: missing "%s"' % (s['n'], key))
        for f in s['facts']:
            if not (isinstance(f, list) and len(f) == 2 and f[0] in FACT):
                fail('stage %s: each fact is [kind, text], kind in %s' % (s['n'], FACT))
        # ids of facts, and the user's "not needed" decisions (overrides.json, never written by Claude)
        s['fids'] = [common.fact_id(s['n'], f[1]) for f in s['facts']]
        s['reasons'] = {}
        for i, fid in enumerate(s['fids']):
            o = (overrides or {}).get(fid)
            if isinstance(o, dict) and o.get('kind') == 'skip' and s['facts'][i][0] not in ('y', 'fixed'):
                s['facts'][i] = ['n', s['facts'][i][1]]
                s['reasons'][str(i)] = o.get('reason', '')
        s['state'] = stage_state(s['facts'])
        s['kind'] = stage_kind(s)
        s['kinds'] = ['fixed' if f[0] == 'fixed' else ('y' if f[0] in DONE else f[0]) for f in s['facts']]
        s['dn'], s['tot'] = sum(1 for f in s['facts'] if f[0] in DONE), len(s['facts'])
    nums = {s['n'] for s in d['stages']}
    d['cur'] = current_stage(d['stages'])

    # blockers must point at real stages and have a reason and an action
    for b in d['blockers']:
        for key in ('t', 'why', 'todo', 'proj', 'stage', 'sev'):
            if key not in b:
                fail('blocker "%s": missing "%s"' % (b.get('t'), key))
        if b['sev'] not in SEV:
            fail('blocker "%s": sev must be one of %s' % (b['t'], SEV))
        if b.get('who') is not None and b['who'] not in WHO:
            fail('blocker "%s": who must be one of %s' % (b['t'], WHO))
        b['bid'] = common.fact_id('b', b['t'])
        if b['stage'] not in nums:
            fail('blocker "%s": stage %s does not exist' % (b['t'], b['stage']))
        for p in b['proj']:
            if p not in projects:
                fail('blocker "%s": unknown project "%s"' % (b['t'], p))

    # ready only when nothing blocks and every stage of the plan is done
    d['ready'] = (not d['blockers']) and bool(d['stages']) and all(s['state'] == 'ok' for s in d['stages'])
    d['pct'] = progress(d['stages'])
    d['kindInfo'] = KINDS
    d.setdefault('updated', datetime.date.today().isoformat())
    return d


def load(src):
    """Read feature.json, apply overrides.json from the same folder, validate and derive."""
    try:
        with open(src, encoding='utf-8') as fh:
            raw = json.load(fh)
    except ValueError as e:
        fail('not valid JSON: %s' % e)
    if not isinstance(raw, dict):
        fail('the top level must be an object')
    overrides = common.read_json(os.path.join(os.path.dirname(src), 'overrides.json'))
    try:
        return prepare(raw, overrides if isinstance(overrides, dict) else {})
    except (AttributeError, IndexError, KeyError, TypeError) as e:
        # a value of the wrong shape (a list where an object is expected, an empty cell, ...)
        fail('wrong shape, see SCHEMA.md (%s: %s)' % (type(e).__name__, e))


def blob(obj):
    """JSON that is safe inside a <script> element: no "<" survives, so neither "</script>" nor
    "<!--" in the data can end or confuse the script."""
    return json.dumps(obj, ensure_ascii=False).replace('<', '\\u003c')


def panel_html(src, back=False, back_href='../index.html', live=False):
    """Validate one feature.json and return (html, data). Nothing is written."""
    data = load(src)
    data['live'] = live
    data['slug'] = os.path.basename(os.path.dirname(os.path.abspath(src)))
    data['ver'] = common.feature_version(data['slug'])
    # the change log is kept only for real features, not for a file rendered from somewhere else
    data['changes'], data['updatedTs'] = track(src, data) if os.path.basename(src) == 'feature.json' else ([], 0)
    data['changes'] = data['changes'][-80:]
    with open(os.path.join(HERE, 'template.html'), encoding='utf-8') as fh:
        html = fh.read()
    html = html.replace('/*__DATA__*/null', blob(data), 1).replace('/*__TITLE__*/', data['title'].replace('<', '&lt;'), 1)
    link = '<a class="back" href="%s">\u2190 All features</a>' % back_href if back else ''
    return html.replace('<!--__BACK__-->', link, 1), data


def write_panel(src, out, back=False):
    html, data = panel_html(src, back)
    with open(out, 'w', encoding='utf-8') as fh:
        fh.write(html)
    return data


def summary(slug, d, today):
    """One row of the portfolio page, derived from a validated feature."""
    try:
        days = (today - datetime.date.fromisoformat(d['updated'])).days
    except ValueError:
        fail('"updated" must be YYYY-MM-DD, got %r' % d['updated'])
    h = sum(1 for b in d['blockers'] if b['sev'] == 'high')
    m = sum(1 for b in d['blockers'] if b['sev'] == 'mid')
    top = next((b for sev in ('high', 'mid', 'low') for b in d['blockers'] if b['sev'] == sev), None)
    done = d['ready']
    return {
        'id': slug, 'name': d['title'], 'projects': d['projects'], 'cur': d['cur'],
        'stages': [{'n': x['n'], 'title': x['title'], 'state': x['state'], 'kind': x['kind'], 'dn': x['dn'], 'tot': x['tot']}
                   for x in d['stages']],
        'pct': d['pct'], 'h': h, 'm': m, 'days': days,
        'stale': days >= 3 and not done, 'done': done,
        'waiting': sum(1 for b in d['blockers'] if b.get('who') == 'you'),
        'why': top['t'] if top else '', 'next': d['nextSteps'][0] if d['nextSteps'] else '',
        'href': '%s/panel.html' % slug,
    }


def collect(root, write=False):
    """Read every <root>/<slug>/feature.json. Returns the payload the portfolio page shows.
    With write=True the per-feature panel.html files are rewritten too."""
    today = datetime.date.today()
    feats, broken = [], []
    for slug in sorted(os.listdir(root)):
        src = os.path.join(root, slug, 'feature.json')
        if not os.path.isfile(src):
            continue
        try:
            if write:
                data = write_panel(src, os.path.join(root, slug, 'panel.html'), back=True)
            else:
                data = load(src)
            feats.append(summary(slug, data, today))
        except (DataError, ValueError, KeyError, TypeError) as e:
            broken.append({'id': slug, 'error': str(e)})
            sys.stderr.write('%s: %s\n' % (slug, e))
    projects = []
    for f in feats:
        for p in f['projects']:
            if p not in projects:
                projects.append(p)
    return {'today': today.strftime('%Y-%m-%d'), 'projects': projects, 'features': feats, 'broken': broken, 'kinds': KINDS}


def index_html(payload):
    with open(os.path.join(HERE, 'template-index.html'), encoding='utf-8') as fh:
        return fh.read().replace('/*__DATA__*/null', blob(payload), 1)


def build_index(root):
    payload = collect(root, write=True)
    out = os.path.join(root, 'index.html')
    with open(out, 'w', encoding='utf-8') as fh:
        fh.write(index_html(payload))
    return out, bool(payload['broken'])


def main():
    argv = sys.argv[1:]
    args = [a for a in argv if not a.startswith('--')]
    try:
        if '--index' in argv:
            root = os.path.abspath(args[0]) if args else common.features_dir()
            if not os.path.isdir(root):
                sys.exit('features folder not found: %s' % root)
            out, bad = build_index(root)
            print(out)
            if '--open' in argv:
                webbrowser.open('file://' + out)
            sys.exit(1 if bad else 0)
        if not args:
            sys.exit(__doc__)
        src = os.path.abspath(args[0])
        out = os.path.join(os.path.dirname(src), 'panel.html')
        if '--out' in argv:
            out = os.path.abspath(argv[argv.index('--out') + 1])
        write_panel(src, out)
    except DataError as e:
        sys.stderr.write('feature.json: %s\n' % e)
        sys.exit(1)
    print(out)
    if '--open' in argv:
        webbrowser.open('file://' + out)


if __name__ == '__main__':
    main()
