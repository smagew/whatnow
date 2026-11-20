#!/usr/bin/env python3
"""Messages typed on the whatnow page. Used by Claude after it handled one.

    python3 inbox.py list [--all]
    python3 inbox.py resolve <id> "one line summary of what was done / the answer"
"""
import os, re, sys, time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common  # noqa: E402


def main():
    a = sys.argv[1:]
    if a[:1] == ['list']:
        for e in common.inbox_entries():
            if '--all' in a or e.get('state') in ('new', 'delivered'):
                print('%s  %-9s %-6s %s: %s' % (e['id'], e.get('state'), e.get('action'), e.get('feature'), e.get('item', '')[:70]))
        return
    if a[:1] == ['resolve'] and len(a) >= 2:
        if not re.match(r'^[0-9a-f]{8,32}$', a[1]):
            sys.exit('bad id')
        path = os.path.join(common.inbox_dir(), '%s.json' % a[1])
        e = common.read_json(path)
        if not e:
            sys.exit('no such message: %s' % a[1])
        e.update(state='done', note=' '.join(a[2:])[:600], doneAt=int(time.time() * 1000))
        common.save_entry(e)
        print('resolved %s' % a[1])
        return
    sys.exit(__doc__)


if __name__ == '__main__':
    main()
