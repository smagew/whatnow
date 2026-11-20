"""Settings shared by render.py, server.py and autostart.py.

Everything is resolved from the environment and ~/.whatnow/config.json, never from a fixed path:

  home          $WHATNOW_HOME, else ~/.whatnow
  featuresDir   $FEATURES_DIR, else "featuresDir" in <home>/config.json, else <home>/features
  port          "port" in <home>/config.json, else 7777
  aliases       "aliases" in <home>/config.json: {repo folder name: project name used in feature.json}
"""
import hashlib, json, os, shutil, tempfile, time

DEFAULT_PORT = 7777


def home():
    return os.environ.get('WHATNOW_HOME') or os.path.expanduser('~/.whatnow')


def config():
    try:
        with open(os.path.join(home(), 'config.json'), encoding='utf-8') as fh:
            data = json.load(fh)
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def features_dir():
    raw = os.environ.get('FEATURES_DIR') or config().get('featuresDir') or os.path.join(home(), 'features')
    return os.path.abspath(os.path.expanduser(raw))


def port():
    try:
        return int(config().get('port', DEFAULT_PORT))
    except (TypeError, ValueError):
        return DEFAULT_PORT


# ---------------------------------------------------------------- paths

def sessions_dir():
    return os.path.join(home(), 'sessions')


def inbox_dir():
    return os.path.join(home(), 'inbox')


def claude_bin():
    """Path of the claude executable. A launchd/systemd service has a short PATH, so besides
    $PATH a few usual install folders are tried. "claudeBin" in config.json wins."""
    configured = config().get('claudeBin')
    if configured and os.path.exists(os.path.expanduser(configured)):
        return os.path.expanduser(configured)
    found = shutil.which('claude')
    if found:
        return found
    for c in ('~/.local/bin/claude', '~/.claude/local/claude', '/usr/local/bin/claude', '/opt/homebrew/bin/claude'):
        c = os.path.expanduser(c)
        if os.path.exists(c):
            return c
    return None


# ---------------------------------------------------------------- features

def feature_index():
    """Light list of features for matching: [{id, title, projects}]. Never raises."""
    out = []
    root = features_dir()
    try:
        names = sorted(os.listdir(root))
    except OSError:
        return out
    for slug in names:
        src = os.path.join(root, slug, 'feature.json')
        try:
            with open(src, encoding='utf-8') as fh:
                d = json.load(fh)
            out.append({'id': slug, 'title': d.get('title', slug), 'projects': list(d.get('projects', []))})
        except (OSError, ValueError, AttributeError):
            continue
    return out


def bindings_path():
    return os.path.join(home(), 'bindings.json')


def bindings():
    """{sessionId: feature slug}: which session works on which feature."""
    d = read_json(bindings_path())
    return d if isinstance(d, dict) else {}


def feature_of(session_id):
    return bindings().get(session_id)


def bind(session_id, slug):
    """Link a session to a feature (a session works on one feature at a time; the last one wins).
    Returns True when something changed."""
    if not session_id or not slug:
        return False
    b = bindings()
    if b.get(session_id) == slug:
        return False
    b[session_id] = slug
    atomic_write_json(bindings_path(), b)
    return True


def slug_in(text, feats):
    """The feature whose folder inside the features directory is mentioned in `text` (a path or a
    shell command); the longest folder name wins. None when no feature is mentioned."""
    if not isinstance(text, str) or not text:
        return None
    root = features_dir().rstrip('/')
    variants = {root, os.path.realpath(root)}
    home_dir = os.path.expanduser('~')
    if root.startswith(home_dir):
        variants.add('~' + root[len(home_dir):])
    best = None
    for f in feats:
        for r in variants:
            if '%s/%s/' % (r, f['id']) in text or '%s/%s"' % (r, f['id']) in text or text.endswith('%s/%s' % (r, f['id'])):
                if best is None or len(f['id']) > len(best):
                    best = f['id']
    return best


# ---------------------------------------------------------------- small helpers

def atomic_write_json(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix='.tmp-')
    with os.fdopen(fd, 'w', encoding='utf-8') as fh:
        json.dump(obj, fh, ensure_ascii=False)
    os.replace(tmp, path)


def read_json(path, default=None):
    try:
        with open(path, encoding='utf-8') as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return {} if default is None else default


def fact_id(stage_n, text):
    """Stable id of one fact: survives reordering, changes only when the text changes."""
    return hashlib.sha1(('%s|%s' % (stage_n, text)).encode('utf-8')).hexdigest()[:10]


# ---------------------------------------------------------------- inbox (messages from the page)

ACTIONS = {'do': 'Task: do this item', 'ask': 'Question: answer the user in this session'}


def format_message(e):
    """The text Claude sees when a message from the page is delivered."""
    return '\n'.join([
        '[whatnow] Message from the dashboard for feature "%s".' % e.get('featureTitle', e.get('feature')),
        'Stage %s: %s' % (e.get('stage'), e.get('stageTitle', '')),
        'Item: %s' % e.get('item', ''),
        'Action: %s' % ACTIONS.get(e.get('action'), e.get('action')),
        'User note: %s' % (e.get('text') or 'none'),
        'When you are done: update the feature.json of feature "%s", rebuild the dashboard '
        '(python3 ~/.claude/skills/whatnow/render.py --index) and mark the message resolved: '
        'python3 ~/.claude/skills/whatnow/inbox.py resolve %s "one-line summary".' % (e.get('feature'), e.get('id')),
    ])


def inbox_entries():
    out = []
    d = inbox_dir()
    try:
        names = os.listdir(d)
    except OSError:
        return out
    for n in names:
        if n.endswith('.json'):
            e = read_json(os.path.join(d, n))
            if e.get('id'):
                out.append(e)
    out.sort(key=lambda e: e.get('ts', 0))
    return out


def save_entry(e):
    atomic_write_json(os.path.join(inbox_dir(), '%s.json' % e['id']), e)


def claim(session_id, slug):
    """Atomically take every undelivered message addressed to this session (or to its feature when
    no session was named). Two watchers can never deliver the same message: the lock file is
    created exclusively. Returns the claimed entries, already marked delivered."""
    got = []
    for e in inbox_entries():
        if e.get('state') != 'new':
            continue
        mine = e.get('session') == session_id or (not e.get('session') and e.get('feature') == slug)
        if not mine:
            continue
        lock = os.path.join(inbox_dir(), '%s.lock' % e['id'])
        try:
            os.close(os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY))
        except OSError:
            continue
        e = read_json(os.path.join(inbox_dir(), '%s.json' % e['id'])) or e
        if e.get('state') != 'new':
            continue
        e.update(state='delivered', session=session_id, deliveredAt=int(time.time() * 1000))
        save_entry(e)
        bind(session_id, e.get('feature'))     # a session that took a message works on that feature
        got.append(e)
    return got
