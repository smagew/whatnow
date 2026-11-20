#!/usr/bin/env python3
"""Render a feature panel (standalone HTML) from a feature.json file.

Usage: python3 render.py path/to/feature.json [--out panel.html] [--open]
       python3 render.py --index [features-dir] [--open]   (all panels + portfolio page)
Validates the data, derives statuses, and fails with a clear message if the
file breaks the rules in SCHEMA.md. Python 3 standard library only.
"""
import json, sys, os, subprocess, datetime

HERE = os.path.dirname(os.path.abspath(__file__))
FACT = ('y', 'bad', 'p', 'q')            # done, problem/rework, partial, not done
LAD = ('ok', 'part', 'no')
CELL = ('ok', 'part', 'no', 'na')
SEV = ('high', 'mid', 'low')
STEPS = ['Написан', 'Тесты', 'Код-ревью', 'Проверен руками', 'На staging', 'На production']


class DataError(Exception):
    pass


def fail(msg):
    raise DataError(msg)


def stage_state(facts):
    kinds = [f[0] for f in facts]
    if kinds and all(k == 'q' for k in kinds):
        return 'no'
    if 'bad' in kinds:
        return 'bad'
    if 'p' in kinds or 'q' in kinds:
        return 'part'
    return 'ok'


def prepare(d):
    for key in ('title', 'projects', 'ladder', 'stages'):
        if key not in d:
            fail('missing "%s"' % key)
    projects = d['projects']
    if not projects or not all(isinstance(p, str) for p in projects):
        fail('"projects" must be a non-empty list of names')
    d.setdefault('sub', '')
    d.setdefault('summary', '')
    for k in ('rollback', 'checks', 'decisions', 'questions', 'blockers', 'nextSteps', 'deploy'):
        d.setdefault(k, [])
    d['ladderSteps'] = STEPS

    # ladder: "ok" | ["ok", "note"] per step, six steps per project
    notes = {}
    for p in projects:
        row = d['ladder'].get(p)
        if not isinstance(row, list) or len(row) != len(STEPS):
            fail('ladder["%s"] needs %d entries (%s)' % (p, len(STEPS), ', '.join(STEPS)))
        clean = []
        for i, c in enumerate(row):
            s, note = (c, None) if isinstance(c, str) else (c[0], c[1] if len(c) > 1 else None)
            if s not in LAD:
                fail('ladder["%s"][%d]: status must be one of %s' % (p, i, LAD))
            if s == 'ok' and i >= 2 and not note:
                fail('ladder["%s"][%d] ("%s") is "ok" without evidence: add a note saying what was checked' % (p, i, STEPS[i]))
            clean.append(s)
            if note:
                notes['%s:%d' % (p, i)] = note
        d['ladder'][p] = clean
    d['ladderNotes'] = notes

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
        s['state'] = stage_state(s['facts'])
    nums = {s['n'] for s in d['stages']}

    # deploy: items are text (not done) or {"t": text, "s": kind}; counts derived
    dTotal = dDone = 0
    for st in d['deploy']:
        items = []
        for it in st['items']:
            it = {'t': it, 's': 'q'} if isinstance(it, str) else it
            if it.get('s') not in FACT:
                fail('deploy step %s: item status must be one of %s' % (st.get('n'), FACT))
            items.append(it)
        st['items'] = items
        st['total'] = len(items)
        st['done'] = sum(1 for i in items if i['s'] == 'y')
        if any(i['s'] == 'bad' for i in items):
            st['cls'], st['tag'], st['stateText'] = 'bad', 'bad', 'есть проблема'
        elif st['done'] == st['total'] and st['total']:
            st['cls'], st['tag'], st['stateText'] = 'ok', 'ok', 'готово'
        elif st['done'] == 0:
            st['cls'], st['tag'], st['stateText'] = 'no', 'no', 'не начато'
        else:
            st['cls'], st['tag'], st['stateText'] = 'part', 'part', 'в процессе'
        dTotal += st['total']
        dDone += st['done']
    d['dTotal'], d['dDone'] = dTotal, dDone

    # checks: a cell is [status, label, note]
    for c in d['checks']:
        for p in projects:
            cell = c.get('cells', {}).get(p)
            if not (isinstance(cell, list) and len(cell) == 3 and cell[0] in CELL):
                fail('check "%s": cells["%s"] must be [status, label, note], status in %s' % (c.get('name'), p, CELL))

    # blockers must point at real stages and have a reason and an action
    for b in d['blockers']:
        for key in ('t', 'why', 'todo', 'proj', 'stage', 'sev'):
            if key not in b:
                fail('blocker "%s": missing "%s"' % (b.get('t'), key))
        if b['sev'] not in SEV:
            fail('blocker "%s": sev must be one of %s' % (b['t'], SEV))
        if b['stage'] not in nums:
            fail('blocker "%s": stage %s does not exist' % (b['t'], b['stage']))
        for p in b['proj']:
            if p not in projects:
                fail('blocker "%s": unknown project "%s"' % (b['t'], p))

    # ready only when nothing blocks and every rung is passed in every project
    d['ready'] = (not d['blockers']) and all(c == 'ok' for p in projects for c in d['ladder'][p])
    d.setdefault('updated', datetime.date.today().isoformat())
    return d


def write_panel(src, out, back=False):
    with open(src, encoding='utf-8') as fh:
        data = prepare(json.load(fh))
    blob = json.dumps(data, ensure_ascii=False).replace('</', '<\\/')
    with open(os.path.join(HERE, 'template.html'), encoding='utf-8') as fh:
        html = fh.read()
    html = html.replace('/*__DATA__*/null', blob, 1).replace('/*__TITLE__*/', data['title'].replace('<', '&lt;'), 1)
    html = html.replace('<!--__BACK__-->', '<a class="back" href="../index.html">← Все функции</a>' if back else '', 1)
    with open(out, 'w', encoding='utf-8') as fh:
        fh.write(html)
    return data


def summary(slug, d, today):
    """One row of the portfolio page, derived from a validated feature."""
    got = tot = 0
    for p in d['projects']:
        for c in d['ladder'][p]:
            tot += 1
            got += {'ok': 1, 'part': .5, 'no': 0}[c]
    try:
        days = (today - datetime.date.fromisoformat(d['updated'])).days
    except ValueError:
        fail('"updated" must be YYYY-MM-DD, got %r' % d['updated'])
    h = sum(1 for b in d['blockers'] if b['sev'] == 'high')
    m = sum(1 for b in d['blockers'] if b['sev'] == 'mid')
    top = next((b for sev in ('high', 'mid', 'low') for b in d['blockers'] if b['sev'] == sev), None)
    done = d['ready']
    return {
        'id': slug, 'name': d['title'], 'projects': d['projects'], 'ladder': d['ladder'],
        'pct': round(100 * got / tot), 'h': h, 'm': m, 'days': days,
        'stale': days >= 3 and not done, 'done': done,
        'why': top['t'] if top else '', 'next': d['nextSteps'][0] if d['nextSteps'] else '',
        'href': '%s/panel.html' % slug,
    }


def build_index(root):
    today = datetime.date.today()
    feats, broken = [], []
    for slug in sorted(os.listdir(root)):
        src = os.path.join(root, slug, 'feature.json')
        if not os.path.isfile(src):
            continue
        try:
            data = write_panel(src, os.path.join(root, slug, 'panel.html'), back=True)
            feats.append(summary(slug, data, today))
        except (DataError, ValueError, KeyError, TypeError) as e:
            broken.append({'id': slug, 'error': str(e)})
            sys.stderr.write('%s: %s\n' % (slug, e))
    projects = []
    for f in feats:
        for p in f['projects']:
            if p not in projects:
                projects.append(p)
    payload = {'today': today.strftime('%d.%m.%Y'), 'projects': projects, 'features': feats, 'broken': broken}
    blob = json.dumps(payload, ensure_ascii=False).replace('</', '<\\/')
    with open(os.path.join(HERE, 'template-index.html'), encoding='utf-8') as fh:
        html = fh.read()
    out = os.path.join(root, 'index.html')
    with open(out, 'w', encoding='utf-8') as fh:
        fh.write(html.replace('/*__DATA__*/null', blob, 1))
    return out, bool(broken)


def main():
    argv = sys.argv[1:]
    args = [a for a in argv if not a.startswith('--')]
    try:
        if '--index' in argv:
            root = os.path.abspath(args[0] if args else os.environ.get('FEATURES_DIR', os.path.expanduser('~/www/features')))
            if not os.path.isdir(root):
                sys.exit('features folder not found: %s' % root)
            out, bad = build_index(root)
            print(out)
            if '--open' in argv:
                subprocess.call(['open', out])
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
        subprocess.call(['open', out])


if __name__ == '__main__':
    main()
