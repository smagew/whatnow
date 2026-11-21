#!/usr/bin/env python3
"""whatnow hook for Claude Code.

Registered for several hook events (see README). Claude Code passes one JSON object on stdin.
This script (1) keeps the live state of each session in <home>/sessions/<session_id>.json and
(2) appends interesting Bash events (git commit, git push, test runs) to <home>/events.jsonl.

It never prints, never blocks, and always exits 0, so a bug here cannot disturb a session.
<home> is $WHATNOW_HOME or ~/.whatnow.

Two more modes deliver messages typed on the whatnow page (see inbox in common.py):
  --deliver  (UserPromptSubmit, synchronous) prints pending messages for this session; Claude Code
             adds that stdout to the context of the prompt being sent.
  --watch    (Stop and SessionStart, asyncRewake) waits in the background while the session is idle
             and, when a message arrives, exits with code 2: Claude Code then wakes the idle session
             and shows stderr to Claude as a system reminder.
"""
import json, os, re, signal, subprocess, sys, tempfile, time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

HOME = os.environ.get('WHATNOW_HOME') or os.path.expanduser('~/.whatnow')
MAX_LOG = 5 * 1024 * 1024

COMMIT = re.compile(r'(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?commit\b')
PUSH = re.compile(r'(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?push\b')
TEST = re.compile(r'\b(phpunit|pytest|unittest|jest|vitest|go test|cargo test|npm (?:run )?test|yarn test|pnpm test|mvn test|gradle test)\b')
NOT_A_RUN = {'pip', 'pip3', 'pipx', 'brew', 'apt', 'apt-get', 'git', 'grep', 'egrep', 'rg', 'ag', 'cat', 'bat', 'ls',
             'find', 'fd', 'echo', 'printf', 'head', 'tail', 'less', 'sed', 'awk', 'wc', 'which', 'type', 'man',
             'cd', 'mkdir', 'touch', 'rm', 'cp', 'mv', 'code', 'vim', 'nano', 'open'}
INSTALL = {'install', 'add', 'remove', 'uninstall', 'i'}


def is_test_run(cmd):
    """True when the command really runs a test tool. A command that only mentions one
    (pip install pytest, grep unittest, git commit -m "fix jest") is not a test run."""
    for part in re.split(r'[;&|\n]+', cmd):
        m = TEST.search(part)
        if not m:
            continue
        before = [w for w in part[:m.start()].split() if not re.match(r'^\w+=', w)]
        first = os.path.basename(before[0].lstrip('(')) if before else ''
        if first in NOT_A_RUN or INSTALL.intersection(before):
            continue
        return True
    return False


def git(cwd, *args):
    try:
        out = subprocess.run(['git', '-C', cwd] + list(args), capture_output=True, text=True, timeout=1.5)
        return out.stdout.strip()
    except Exception:
        return ''


def commit_message(cmd):
    m = re.search(r"-m\s+(?:\"([^\"]*)\"|'([^']*)')", cmd)
    if m:
        text = m.group(1) or m.group(2) or ''
        if text.startswith('$(cat'):
            text = ''
    else:
        text = ''
    if not text:
        h = re.search(r"<<-?\s*'?(\w+)'?\s*\n(.*?)\n", cmd, re.S)
        text = h.group(2) if h else ''
    return ' '.join(text.split())[:90]


def test_summary(stdout):
    ran = re.search(r'Ran (\d+) tests?', stdout or '')        # python unittest
    if ran:
        if 'FAILED' in stdout:
            bad = sum(int(n) for n in re.findall(r'(?:failures|errors)=(\d+)', stdout))
            return 'tests failed: %d of %s' % (bad, ran.group(1)) if bad else 'tests failed'
        if re.search(r'^OK\b', stdout, re.M):
            return 'tests passed: %s' % ran.group(1)
    m = re.search(r'OK \((\d+) tests?', stdout or '')
    if m:
        return 'tests passed: %s' % m.group(1)
    m = re.search(r'Tests:\s*(\d+)(?:,\s*Assertions:\s*\d+)?(?:,\s*Failures:\s*(\d+))?', stdout or '')
    if m and m.group(2):
        return 'tests failed: %s of %s' % (m.group(2), m.group(1))
    # pytest / jest / vitest: "3 failed, 10 passed", "2 errors in 0.5s". Failures first: a pipe
    # (pytest | tail) hides the exit code, so the text is all we have.
    bad = sum(int(n) for n in re.findall(r'\b(\d+) (?:failed\b|errors? in \d)', stdout or ''))
    m = re.search(r'(\d+) passed', stdout or '')
    if bad:
        return 'tests failed: %d of %d' % (bad, bad + int(m.group(1))) if m else 'tests failed: %d' % bad
    if m:
        return 'tests passed: %s' % m.group(1)
    return ''


def classify(payload, failed):
    cmd = (payload.get('tool_input') or {}).get('command') or ''
    resp = payload.get('tool_response') or {}
    stdout = ''
    if isinstance(resp, dict):
        stdout = '%s\n%s' % (resp.get('stdout') or '', resp.get('stderr') or '')    # unittest prints to stderr
    err = payload.get('error') or ''
    code = ''
    m = re.match(r'Exit code (\d+)', err)
    if m:
        code = m.group(1)
    if COMMIT.search(cmd) and not failed:
        msg = commit_message(cmd)
        return 'commit', True, 'commit' + (' "%s"' % msg if msg else '')
    if PUSH.search(cmd):
        return 'push', not failed, 'push to origin' if not failed else 'push failed'
    if is_test_run(cmd):
        txt = test_summary(stdout) or test_summary(err)
        if failed or txt.startswith('tests failed'):
            return 'tests', False, txt or ('tests failed' + (' (exit %s)' % code if code else ''))
        return 'tests', True, txt or 'tests passed'
    return None, None, ''


def read_json(path):
    try:
        with open(path, encoding='utf-8') as fh:
            return json.load(fh)
    except Exception:
        return {}


def write_json_atomic(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix='.tmp-')
    with os.fdopen(fd, 'w', encoding='utf-8') as fh:
        json.dump(obj, fh, ensure_ascii=False)
    os.replace(tmp, path)


def append_event(ev):
    path = os.path.join(HOME, 'events.jsonl')
    os.makedirs(HOME, exist_ok=True)
    line = (json.dumps(ev, ensure_ascii=False) + '\n').encode('utf-8')
    fd = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_CREAT, 0o644)
    try:
        os.write(fd, line)          # one write: lines of parallel sessions do not interleave
    finally:
        os.close(fd)
    try:
        if os.path.getsize(path) > MAX_LOG:
            with open(path, 'rb') as fh:
                tail = fh.read().splitlines()[-4000:]
            write_json_atomic_raw(path, b'\n'.join(tail) + b'\n')
    except Exception:
        pass


def write_json_atomic_raw(path, data):
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix='.tmp-')
    with os.fdopen(fd, 'wb') as fh:
        fh.write(data)
    os.replace(tmp, path)


def is_watcher(pid):
    """True when this pid is one of our --watch processes. A pid file outlives a reboot or a kill -9,
    and the number may by then belong to some other program, which must not get our SIGTERM."""
    try:
        out = subprocess.run(['ps', '-p', str(pid), '-o', 'command='], capture_output=True, text=True, timeout=2).stdout
        return 'hook.py' in out and '--watch' in out
    except Exception:
        return False


def read_pid(path):
    try:
        with open(path) as fh:
            return int(fh.read().strip())
    except (OSError, ValueError):
        return 0


WRITE_TOOLS = ('Edit', 'Write', 'MultiEdit', 'NotebookEdit')
SKILL_CMD = re.compile(r'(?:render|inbox)\.py([^|;&\n]*)')


def auto_bind(sid, tool_name, tool_input):
    """A session that WORKS on a feature is linked to it: it edits the feature's files, or runs the
    skill's render.py / inbox.py on it. Reading or listing the files does not count."""
    try:
        import common
        if not isinstance(tool_input, dict):
            return
        feats = common.feature_index()
        if tool_name in WRITE_TOOLS:
            texts = [tool_input.get('file_path'), tool_input.get('notebook_path')]
        elif tool_name == 'Bash':
            # only the arguments of render.py / inbox.py count, not a path elsewhere in the command line
            texts = [m.group(1) for m in SKILL_CMD.finditer(tool_input.get('command') or '')]
        else:
            return
        for text in texts:
            slug = common.slug_in(text, feats)
            if slug:
                common.bind(sid, slug)
                return
    except Exception:
        pass


def deliver(payload):
    """UserPromptSubmit: print the messages waiting for this session. Printed text reaches Claude."""
    import common
    sid = payload.get('session_id') or ''
    got = common.claim(sid, common.feature_of(sid))
    if got:
        sys.stdout.write('\n\n'.join(common.format_message(e) for e in got) + '\n')
    return 0


def watch(payload):
    """Stop / SessionStart: wait while the session is idle; exit 2 when a message arrives (wakes Claude)."""
    import common
    sid = payload.get('session_id') or ''
    slug = common.feature_of(sid)          # may be None: messages can also be addressed to the session itself
    if not sid:
        return 0
    wdir = os.path.join(HOME, 'watch')
    os.makedirs(wdir, exist_ok=True)
    pidf = os.path.join(wdir, '%s.pid' % re.sub(r'[^A-Za-z0-9_-]', '_', sid))
    try:
        old = read_pid(pidf)
        if old and old != os.getpid() and is_watcher(old):
            os.kill(old, signal.SIGTERM)          # one watcher per session
    except OSError:
        pass
    with open(pidf, 'w') as fh:
        fh.write(str(os.getpid()))
    spath = os.path.join(HOME, 'sessions', '%s.json' % re.sub(r'[^A-Za-z0-9_-]', '_', sid))
    start = time.time()
    deadline = start + float(os.environ.get('WHATNOW_WATCH_SECONDS', 3300))   # below the hook timeout (3600)
    try:
        while time.time() < deadline:
            got = common.claim(sid, slug)
            if got:
                sys.stderr.write('\n\n'.join(common.format_message(e) for e in got) + '\n')
                sys.stderr.flush()
                return 2
            st = read_json(spath)
            if st.get('state') == 'closed':
                return 0
            if time.time() - start > 5 and st.get('state') == 'work' and st.get('t', 0) > (start + 3) * 1000:
                return 0                  # the user typed: the synchronous hook delivers at that prompt
            if os.getppid() == 1:
                return 0                  # Claude Code is gone
            time.sleep(float(os.environ.get('WHATNOW_WATCH_POLL', 2)))
        return 0
    finally:
        try:
            if read_pid(pidf) == os.getpid():
                os.remove(pidf)
        except OSError:
            pass


NUDGE_COOLDOWN = 1800                             # seconds between reminders to one session


def nudge(payload):
    """Stop: a session linked to a feature that finished a turn without touching the feature's file is
    reminded, once per user prompt, to bring the dashboard in line with what it just told the user.
    The reminder (stderr, exit code 2) wakes the session. `\"nudge\": false` in config.json turns it off."""
    import common
    sid = payload.get('session_id') or ''
    if payload.get('stop_hook_active'):
        return 0                                  # this turn was itself caused by a stop hook: never chain
    slug = common.feature_of(sid)
    if not slug or common.config().get('nudge') is False:
        return 0
    key = re.sub(r'[^A-Za-z0-9_-]', '_', sid)
    st = read_json(os.path.join(HOME, 'sessions', '%s.json' % key))
    prompt_t = st.get('promptT') or 0
    if not prompt_t:
        return 0
    try:
        updated = os.stat(os.path.join(common.features_dir(), slug, 'feature.json')).st_mtime * 1000
    except OSError:
        return 0
    if updated >= prompt_t:
        return 0                                  # the feature file was updated during this turn
    mark = os.path.join(HOME, 'nudged', key)
    os.makedirs(os.path.dirname(mark), exist_ok=True)
    try:
        with open(mark) as fh:
            seen, _, at = fh.read().strip().partition(' ')
        if seen == str(prompt_t):
            return 0                              # already reminded for this prompt: never loop
        if at and time.time() - float(at) < NUDGE_COOLDOWN:
            return 0                              # the reminder itself counts as a prompt: wait before the next
    except (OSError, ValueError):
        pass
    with open(mark, 'w') as fh:
        fh.write('%s %s' % (prompt_t, time.time()))
    sys.stderr.write(
        '[whatnow] You finished a turn on feature "%s" without updating its dashboard. If you told the user anything '
        'about status, open items, results or what is left, make feature.json match it now (set "who" on every blocker, '
        'keep evidence for anything marked done), then run python3 ~/.claude/skills/whatnow/render.py --index. '
        'If nothing changed, reply with just "dashboard in sync".\n' % slug)
    return 2


def main():
    payload = json.load(sys.stdin)
    event = payload.get('hook_event_name') or ''
    sid = payload.get('session_id') or ''
    cwd = payload.get('cwd') or os.getcwd()
    if not sid:
        return
    if '--deliver' in sys.argv:
        return deliver(payload)
    if '--watch' in sys.argv:
        return watch(payload)
    if '--nudge' in sys.argv:
        return nudge(payload)
    spath = os.path.join(HOME, 'sessions', '%s.json' % re.sub(r'[^A-Za-z0-9_-]', '_', sid))

    if event == 'SessionEnd':
        prev = read_json(spath)
        if prev:
            prev.update(state='closed', what='session closed', t=int(time.time() * 1000))
            write_json_atomic(spath, prev)
        return

    top = git(cwd, 'rev-parse', '--show-toplevel')
    branch = git(cwd, 'branch', '--show-current') or git(cwd, 'rev-parse', '--short', 'HEAD')
    prev = read_json(spath)       # read after the slow git calls: a parallel hook may have written meanwhile
    state = {
        'session': sid, 'cwd': cwd,
        'project': os.path.basename(top or cwd.rstrip('/')),
        'branch': branch,
        'state': prev.get('state', 'idle'), 'what': prev.get('what', ''), 't': int(time.time() * 1000),
        'promptT': prev.get('promptT', 0),
    }

    if event == 'SessionStart':
        state.update(state='idle', what='session started')
    elif event == 'UserPromptSubmit':
        state.update(state='work', what='received a prompt', promptT=state['t'])
    elif event in ('PostToolUse', 'PostToolUseFailure'):
        if payload.get('tool_name') == 'Bash':
            kind, ok, text = classify(payload, event == 'PostToolUseFailure')
            state.update(state='work', what=text or 'running commands')
            if kind:
                append_event({'ts': state['t'], 'session': sid, 'project': state['project'],
                              'branch': state['branch'], 'kind': kind, 'ok': ok, 'text': text})
        else:
            state.update(state='work')
        auto_bind(sid, payload.get('tool_name'), payload.get('tool_input'))
    elif event == 'Stop':
        state.update(state='wait', what='answered, waiting for you')
    elif event == 'Notification':
        ntype = payload.get('notification_type') or ''
        if ntype == 'permission_prompt':
            state.update(state='wait', what='waiting for permission')
        elif ntype == 'idle_prompt':
            state.update(state='idle', what='idle')
        else:
            return
    else:
        return
    write_json_atomic(spath, state)


if __name__ == '__main__':
    rc = 0
    try:
        rc = main() or 0
    except BaseException:
        rc = 0
    sys.exit(rc)
