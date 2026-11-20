#!/usr/bin/env python3
"""whatnow server: the portfolio page that updates itself.

    python3 server.py [--port 7777] [--open]

Serves http://127.0.0.1:<port>/ . The page polls /api/state every few seconds. On each request
the server re-reads every feature.json and the hook data, so nothing has to be rebuilt.

Config: see common.py (<home>/config.json, home = $WHATNOW_HOME or ~/.whatnow).
Only listens on 127.0.0.1 and rejects foreign Host headers.
"""
import json, os, re, subprocess, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import common  # noqa: E402
import render  # noqa: E402

HOME = common.home()
WORK_STALE = 10 * 60 * 1000       # "работает" without a single event for 10 min is not trusted
DROP_AFTER = 6 * 3600 * 1000      # a session silent for 6 h is not shown
SLUG = re.compile(r'^[A-Za-z0-9._-]+$')


config = common.config
features_dir = common.features_dir


def match_feature(feats, project, branch):
    """A session belongs to a feature when its branch is listed in feature.json "branches",
    or when the feature slug is part of the branch name."""
    b = (branch or '').lower()
    for f in feats:
        if branch in f.get('branches', []):
            return f['id']
    best = None
    for f in feats:
        slug = f['id'].lower()
        if slug and slug in b and project in f['projects']:
            if best is None or len(slug) > len(best):
                best = slug
    return next((f['id'] for f in feats if f['id'].lower() == best), None)


def read_sessions(feats, now):
    aliases = config().get('aliases', {})
    out = []
    d = os.path.join(HOME, 'sessions')
    if not os.path.isdir(d):
        return out
    for name in os.listdir(d):
        if not name.endswith('.json'):
            continue
        try:
            with open(os.path.join(d, name), encoding='utf-8') as fh:
                s = json.load(fh)
        except Exception:
            continue
        age = now - int(s.get('t', 0))
        if age > DROP_AFTER:
            continue
        state = s.get('state', 'idle')
        if state == 'work' and age > WORK_STALE:
            state = 'idle'
        project = aliases.get(s.get('project'), s.get('project'))
        out.append({'id': s.get('session', name), 'project': project, 'branch': s.get('branch', ''),
                    'state': state, 'what': s.get('what', ''), 't': s.get('t', now),
                    'feature': match_feature(feats, project, s.get('branch'))})
    out.sort(key=lambda x: -x['t'])
    return out


def read_events(feats, now, limit=30):
    aliases = config().get('aliases', {})
    path = os.path.join(HOME, 'events.jsonl')
    by = {}
    try:
        with open(path, 'rb') as fh:
            lines = fh.read().splitlines()[-3000:]
    except OSError:
        return by
    for line in reversed(lines):
        try:
            e = json.loads(line)
        except Exception:
            continue
        project = aliases.get(e.get('project'), e.get('project'))
        slug = match_feature(feats, project, e.get('branch'))
        if not slug:
            continue
        lst = by.setdefault(slug, [])
        if len(lst) < limit:
            k = 'in' if e.get('kind') in ('commit', 'push') else ('ok' if e.get('ok') else 'bad')
            lst.append({'k': k, 'p': project, 't': e.get('text', ''), 'ts': e.get('ts', 0)})
    return by


def state():
    now = int(time.time() * 1000)
    payload = render.collect(features_dir())
    for f in payload['features']:
        f['href'] = '/f/%s/' % f['id']
    payload['live'] = True
    payload['now'] = now
    payload['sessions'] = read_sessions(payload['features'], now)
    payload['events'] = read_events(payload['features'], now)
    return payload


class Handler(BaseHTTPRequestHandler):
    server_version = 'whatnow'

    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype):
        data = body.encode('utf-8') if isinstance(body, str) else body
        self.send_response(code)
        self.send_header('Content-Type', ctype)
        self.send_header('Content-Length', str(len(data)))
        self.send_header('Cache-Control', 'no-store')
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        host = (self.headers.get('Host') or '').split(':')[0]
        if host not in ('127.0.0.1', 'localhost'):
            return self._send(403, 'forbidden', 'text/plain; charset=utf-8')
        path = self.path.split('?')[0]
        try:
            if path == '/':
                return self._send(200, render.index_html(state()), 'text/html; charset=utf-8')
            if path == '/api/state':
                return self._send(200, json.dumps(state(), ensure_ascii=False), 'application/json; charset=utf-8')
            m = re.match(r'^/f/([^/]+)/?$', path)
            if m and SLUG.match(m.group(1)):
                src = os.path.join(features_dir(), m.group(1), 'feature.json')
                if os.path.isfile(src):
                    html, _ = render.panel_html(src, back=True, back_href='/')
                    return self._send(200, html, 'text/html; charset=utf-8')
            return self._send(404, 'not found', 'text/plain; charset=utf-8')
        except render.DataError as e:
            return self._send(500, 'feature.json: %s' % e, 'text/plain; charset=utf-8')
        except Exception as e:  # keep the server alive whatever a data file contains
            return self._send(500, 'error: %s' % e, 'text/plain; charset=utf-8')


def main():
    argv = sys.argv[1:]
    port = int(argv[argv.index('--port') + 1]) if '--port' in argv else common.port()
    try:
        srv = ThreadingHTTPServer(('127.0.0.1', port), Handler)
    except OSError as e:
        sys.exit('port %d is not available: %s (use --port)' % (port, e))
    url = 'http://127.0.0.1:%d/' % port
    print('whatnow: %s  (features: %s)' % (url, features_dir()))
    if '--open' in argv:
        subprocess.call(['open', url])
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == '__main__':
    main()
