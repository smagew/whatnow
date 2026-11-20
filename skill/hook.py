#!/usr/bin/env python3
"""whatnow hook for Claude Code.

Registered for several hook events (see README). Claude Code passes one JSON object on stdin.
This script (1) keeps the live state of each session in <home>/sessions/<session_id>.json and
(2) appends interesting Bash events (git commit, git push, test runs) to <home>/events.jsonl.

It never prints, never blocks, and always exits 0, so a bug here cannot disturb a session.
<home> is $WHATNOW_HOME or ~/.whatnow.
"""
import json, os, re, subprocess, sys, tempfile, time

HOME = os.environ.get('WHATNOW_HOME') or os.path.expanduser('~/.whatnow')
MAX_LOG = 5 * 1024 * 1024

COMMIT = re.compile(r'(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?commit\b')
PUSH = re.compile(r'(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?push\b')
TEST = re.compile(r'\b(phpunit|pytest|unittest|jest|vitest|go test|cargo test|npm (?:run )?test|yarn test|pnpm test|mvn test|gradle test)\b')


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
            return 'тесты упали: %d из %s' % (bad, ran.group(1)) if bad else 'тесты упали'
        if re.search(r'^OK\b', stdout, re.M):
            return 'тесты прошли: %s' % ran.group(1)
    m = re.search(r'OK \((\d+) tests?', stdout or '')
    if m:
        return 'тесты прошли: %s' % m.group(1)
    m = re.search(r'Tests:\s*(\d+)(?:,\s*Assertions:\s*\d+)?(?:,\s*Failures:\s*(\d+))?', stdout or '')
    if m and m.group(2):
        return 'тесты упали: %s из %s' % (m.group(2), m.group(1))
    m = re.search(r'(\d+) passed', stdout or '')
    if m:
        return 'тесты прошли: %s' % m.group(1)
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
        return 'commit', True, 'коммит' + (' «%s»' % msg if msg else '')
    if PUSH.search(cmd):
        return 'push', not failed, 'push в origin' if not failed else 'push не удался'
    if TEST.search(cmd):
        txt = test_summary(stdout) or test_summary(err)
        if failed:
            return 'tests', False, txt or ('тесты упали' + (' (код %s)' % code if code else ''))
        return 'tests', True, txt or 'тесты прошли'
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


def main():
    payload = json.load(sys.stdin)
    event = payload.get('hook_event_name') or ''
    sid = payload.get('session_id') or ''
    cwd = payload.get('cwd') or os.getcwd()
    if not sid:
        return
    spath = os.path.join(HOME, 'sessions', '%s.json' % re.sub(r'[^A-Za-z0-9_-]', '_', sid))

    if event == 'SessionEnd':
        try:
            os.remove(spath)
        except OSError:
            pass
        return

    prev = read_json(spath)
    top = git(cwd, 'rev-parse', '--show-toplevel')
    state = {
        'session': sid, 'cwd': cwd,
        'project': os.path.basename(top or cwd.rstrip('/')),
        'branch': git(cwd, 'branch', '--show-current') or git(cwd, 'rev-parse', '--short', 'HEAD'),
        'state': prev.get('state', 'idle'), 'what': prev.get('what', ''), 't': int(time.time() * 1000),
    }

    if event == 'SessionStart':
        state.update(state='idle', what='сессия началась')
    elif event == 'UserPromptSubmit':
        state.update(state='work', what='получил запрос')
    elif event in ('PostToolUse', 'PostToolUseFailure'):
        if payload.get('tool_name') == 'Bash':
            kind, ok, text = classify(payload, event == 'PostToolUseFailure')
            state.update(state='work', what=text or 'выполняет команды')
            if kind:
                append_event({'ts': state['t'], 'session': sid, 'project': state['project'],
                              'branch': state['branch'], 'kind': kind, 'ok': ok, 'text': text})
        else:
            state.update(state='work')
    elif event == 'Stop':
        state.update(state='wait', what='ответил, ждёт вас')
    elif event == 'Notification':
        ntype = payload.get('notification_type') or ''
        if ntype == 'permission_prompt':
            state.update(state='wait', what='ждёт разрешения')
        elif ntype == 'idle_prompt':
            state.update(state='idle', what='простаивает')
        else:
            return
    else:
        return
    write_json_atomic(spath, state)


if __name__ == '__main__':
    try:
        main()
    except BaseException:
        pass
    sys.exit(0)
