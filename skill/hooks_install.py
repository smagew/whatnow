#!/usr/bin/env python3
"""Add or remove the whatnow hooks in a Claude Code settings.json.

    python3 hooks_install.py --install   [--file ~/.claude/settings.json] [--dry-run]
    python3 hooks_install.py --uninstall [--file ...] [--dry-run]

Safe by design: refuses to touch a file that is not valid JSON, keeps every other setting and hook,
is idempotent (running twice changes nothing), makes a timestamped backup next to the file
before writing, and writes atomically. Our entries are recognised by the hook.py command.
"""
import difflib, json, os, shutil, sys, tempfile, time

EVENTS = [('SessionStart', None), ('UserPromptSubmit', None), ('PostToolUse', 'Bash'),
          ('PostToolUseFailure', 'Bash'), ('Stop', None), ('Notification', None), ('SessionEnd', None)]
MARK = 'whatnow/hook.py'
COMMAND = 'python3 ~/.claude/skills/whatnow/hook.py'


def entry(matcher):
    e = {'hooks': [{'type': 'command', 'command': COMMAND, 'async': True}]}
    if matcher:
        e = dict([('matcher', matcher)] + list(e.items()))
    return e


def ours(group):
    return any(MARK in (h.get('command') or '') for h in group.get('hooks', []))


def install(d):
    hooks = d.setdefault('hooks', {})
    for event, matcher in EVENTS:
        groups = hooks.setdefault(event, [])
        if not any(ours(g) for g in groups):
            groups.append(entry(matcher))
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
