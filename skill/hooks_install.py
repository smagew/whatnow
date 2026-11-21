#!/usr/bin/env python3
"""Add or remove the whatnow hooks in a Claude Code settings.json.

    python3 hooks_install.py --install   [--file ~/.claude/settings.json] [--dry-run]
    python3 hooks_install.py --uninstall [--file ...] [--dry-run]
    python3 hooks_install.py --print        (show the hooks block, change nothing)

Safe by design: refuses to touch a file that is not valid JSON, keeps every other setting and hook,
is idempotent (running twice changes nothing), makes a timestamped backup next to the file
before writing, and writes atomically. Our entries are recognised by the hook.py command.
"""
import difflib, json, os, shutil, sys, tempfile, time

STATE = 'python3 ~/.claude/skills/whatnow/hook.py'
MARK = 'whatnow/hook.py'

# Bash for the events, the file tools so that a session that edits a feature.json is linked to that feature
TOOLS = 'Bash|Edit|Write|MultiEdit|NotebookEdit'

# (event, matcher, command, extra handler fields)
ENTRIES = [
    ('SessionStart',       None,   STATE,                 {'async': True}),
    ('SessionStart',       None,   STATE + ' --watch',    {'asyncRewake': True, 'timeout': 3600}),
    ('UserPromptSubmit',   None,   STATE,                 {'async': True}),
    ('UserPromptSubmit',   None,   STATE + ' --deliver',  {'timeout': 10}),
    ('PostToolUse',        TOOLS,  STATE,                 {'async': True}),
    ('PostToolUseFailure', 'Bash', STATE,                 {'async': True}),
    ('Stop',               None,   STATE,                 {'async': True}),
    ('Stop',               None,   STATE + ' --watch',    {'asyncRewake': True, 'timeout': 3600}),
    ('Stop',               None,   STATE + ' --nudge',    {'asyncRewake': True, 'timeout': 30}),
    ('Notification',       None,   STATE,                 {'async': True}),
    ('SessionEnd',         None,   STATE,                 {'async': True}),
]


def group(matcher, command, extra):
    handler = dict([('type', 'command'), ('command', command)] + list(extra.items()))
    g = {'hooks': [handler]}
    if matcher:
        g = dict([('matcher', matcher)] + list(g.items()))
    return g


def desired():
    out = {}
    for event, matcher, command, extra in ENTRIES:
        out.setdefault(event, []).append(group(matcher, command, extra))
    return out


def ours(g):
    return any(MARK in (h.get('command') or '') for h in g.get('hooks', []))


def install(d):
    hooks = d.setdefault('hooks', {})
    for event, want in desired().items():
        groups = hooks.setdefault(event, [])
        if [g for g in groups if ours(g)] != want:
            hooks[event] = [g for g in groups if not ours(g)] + want
    return d


def uninstall(d):
    hooks = d.get('hooks', {})
    for event in list(hooks):
        hooks[event] = [g for g in hooks[event] if not ours(g)]
        if not hooks[event]:
            del hooks[event]
    if 'hooks' in d and not d['hooks']:
        del d['hooks']
    return d


def main():
    argv = sys.argv[1:]
    if '--print' in argv:
        print(json.dumps({'hooks': desired()}, ensure_ascii=False, indent=2))
        return
    if '--install' not in argv and '--uninstall' not in argv:
        sys.exit(__doc__)
    path = os.path.expanduser(argv[argv.index('--file') + 1] if '--file' in argv else '~/.claude/settings.json')
    raw = open(path, encoding='utf-8').read() if os.path.exists(path) else '{}\n'
    try:
        data = json.loads(raw)
    except ValueError as e:
        sys.exit('%s is not valid JSON, nothing changed: %s' % (path, e))
    if not isinstance(data, dict):
        sys.exit('%s: top level must be an object, nothing changed' % path)
    new = (install if '--install' in argv else uninstall)(json.loads(raw))
    indent = 2
    text = json.dumps(new, ensure_ascii=False, indent=indent) + '\n'
    before = json.dumps(data, ensure_ascii=False, indent=indent) + '\n'
    if text == before:
        print('no changes needed')
        return
    diff = ''.join(difflib.unified_diff(before.splitlines(True), text.splitlines(True), 'before', 'after'))
    print(diff)
    if '--dry-run' in argv:
        print('(dry run, nothing written)')
        return
    if os.path.exists(path):
        bak = '%s.bak-whatnow-%s' % (path, time.strftime('%Y%m%d-%H%M%S'))
        shutil.copy2(path, bak)
        print('backup: %s' % bak)
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path) or '.', prefix='.tmp-')
    with os.fdopen(fd, 'w', encoding='utf-8') as fh:
        fh.write(text)
    os.replace(tmp, path)
    print('written: %s' % path)


if __name__ == '__main__':
    main()
