#!/usr/bin/env python3
"""Start the whatnow server automatically when you log in.

    python3 autostart.py --install   [--dry-run]
    python3 autostart.py --uninstall
    python3 autostart.py --status

macOS: a LaunchAgent  (~/Library/LaunchAgents/whatnow.server.plist)
Linux: a systemd user unit  (~/.config/systemd/user/whatnow.service)
Other systems: not supported, start the server by hand (python3 server.py).

Nothing is hard-coded: the Python interpreter is the one running this script (the base one if that
is a virtualenv), the server path is
next to this file, the port and data folder come from common.py (config.json / environment).
Logs go to <home>/server.log. Install is idempotent.
"""
import os, plistlib, subprocess, sys, urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import common  # noqa: E402

LABEL = 'whatnow.server'
UNIT = 'whatnow.service'


def interpreter():
    """The Python to run the service with. Inside a virtualenv we use the interpreter the venv
    was made from (its pyvenv.cfg "home"), because a service must not break when someone deletes
    the environment. Re-run --install after you change your Python."""
    if sys.base_prefix == sys.prefix:
        return sys.executable
    home = None
    try:
        with open(os.path.join(sys.prefix, 'pyvenv.cfg'), encoding='utf-8') as fh:
            for line in fh:
                key, _, value = line.partition('=')
                if key.strip() == 'home':
                    home = value.strip()
    except OSError:
        pass
    candidates = []
    if home:
        candidates += [os.path.join(home, 'python3'), os.path.join(home, 'python')]
    candidates.append(getattr(sys, '_base_executable', ''))
    for c in candidates:
        if c and os.path.exists(c):
            return c
    return sys.executable


def command():
    return [interpreter(), os.path.join(HERE, 'server.py'), '--port', str(common.port())]


def env():
    """Only what the server needs to find its data. Keeps the service independent of your shell."""
    e = {}
    for k in ('WHATNOW_HOME', 'FEATURES_DIR'):
        if os.environ.get(k):
            e[k] = os.environ[k]
    return e


def build_plist(cmd, log, environment):
    d = {'Label': LABEL, 'ProgramArguments': cmd, 'RunAtLoad': True, 'KeepAlive': True,
         'StandardOutPath': log, 'StandardErrorPath': log, 'ProcessType': 'Background'}
    if environment:
        d['EnvironmentVariables'] = environment
    return plistlib.dumps(d).decode('utf-8')


def build_unit(cmd, log, environment):
    q = lambda s: '"%s"' % s.replace('"', '\\"')
    lines = ['[Unit]', 'Description=whatnow server (feature status page)', '',
             '[Service]', 'ExecStart=%s' % ' '.join(q(c) for c in cmd), 'Restart=on-failure', 'RestartSec=5',
             'StandardOutput=append:%s' % log, 'StandardError=append:%s' % log]
    lines += ['Environment=%s' % q('%s=%s' % kv) for kv in sorted(environment.items())]
    lines += ['', '[Install]', 'WantedBy=default.target', '']
    return '\n'.join(lines)


def run(*args, check=True):
    r = subprocess.run(list(args), capture_output=True, text=True)
    if check and r.returncode != 0:
        sys.exit('%s failed: %s' % (' '.join(args), (r.stderr or r.stdout).strip()))
    return r


def paths():
    if sys.platform == 'darwin':
        return os.path.expanduser('~/Library/LaunchAgents/%s.plist' % LABEL)
    if sys.platform.startswith('linux'):
        return os.path.expanduser('~/.config/systemd/user/%s' % UNIT)
    return None


def alive():
    try:
        with urllib.request.urlopen('http://127.0.0.1:%d/api/state' % common.port(), timeout=2) as r:
            return r.status == 200
    except Exception:
        return False


def install(dry):
    path = paths()
    if not path:
        sys.exit('Autostart is not supported on %s. Start the server by hand: python3 server.py' % sys.platform)
    os.makedirs(common.home(), exist_ok=True)
    log = os.path.join(common.home(), 'server.log')
    text = (build_plist if sys.platform == 'darwin' else build_unit)(command(), log, env())
    print('file: %s\n%s' % (path, text))
    if dry:
        print('(dry run, nothing written)')
        return
    if alive():
        print('note: something already answers on port %d (a server started by hand?). Stop it, '
              'otherwise the autostarted one cannot bind the port.' % common.port())
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'w', encoding='utf-8') as fh:
        fh.write(text)
    if sys.platform == 'darwin':
        target = 'gui/%d' % os.getuid()
        run('launchctl', 'bootout', '%s/%s' % (target, LABEL), check=False)
        run('launchctl', 'bootstrap', target, path)
    else:
        run('systemctl', '--user', 'daemon-reload')
        run('systemctl', '--user', 'enable', '--now', UNIT)
    print('installed. Page: http://127.0.0.1:%d/   log: %s' % (common.port(), log))


def uninstall():
    path = paths()
    if not path:
        sys.exit('Autostart is not supported on %s.' % sys.platform)
    if sys.platform == 'darwin':
        run('launchctl', 'bootout', 'gui/%d/%s' % (os.getuid(), LABEL), check=False)
    else:
        run('systemctl', '--user', 'disable', '--now', UNIT, check=False)
    if os.path.exists(path):
        os.remove(path)
        print('removed %s' % path)
    else:
        print('was not installed')
    if sys.platform.startswith('linux'):
        run('systemctl', '--user', 'daemon-reload', check=False)


def status():
    path = paths()
    print('platform: %s' % sys.platform)
    print('autostart file: %s (%s)' % (path, 'exists' if path and os.path.exists(path) else 'absent'))
    print('server answers on port %d: %s' % (common.port(), 'yes' if alive() else 'no'))


def main():
    argv = sys.argv[1:]
    if '--install' in argv:
        install('--dry-run' in argv)
    elif '--uninstall' in argv:
        uninstall()
    elif '--status' in argv:
        status()
    else:
        sys.exit(__doc__)


if __name__ == '__main__':
    main()
