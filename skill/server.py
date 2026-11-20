#!/usr/bin/env python3
"""whatnow server: the portfolio page that updates itself, and the buttons on it.

    python3 server.py [--port 7777] [--open]

Serves http://127.0.0.1:<port>/ . The page polls /api/state every few seconds. On each request
the server re-reads every feature.json and the hook data, so nothing has to be rebuilt.

Buttons on a feature page (Do / Ask / Not needed) post to /api/act:
  skip / unskip  applied at once, stored in <feature>/overrides.json (never in feature.json,
                 which Claude edits, so the two cannot overwrite each other)
  do / ask       written to the inbox (<home>/inbox) and routed:
                   a session of that feature is open  -> addressed to it; it is woken (asyncRewake
                                                         hook) or gets the text at its next prompt
                   only closed sessions are known     -> `claude --resume <id> --bg "<message>"`
                   nothing known                      -> waits and is delivered to the next session
                                                         that works on that feature
Config: see common.py. Only listens on 127.0.0.1, rejects foreign Host/Origin headers.
"""
import json, os, re, subprocess, sys, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import common  # noqa: E402
import render  # noqa: E402

HOME = common.home()
config = common.config
features_dir = common.features_dir
WORK_STALE = 10 * 60 * 1000       # "work" without a single event for 10 min is not trusted
DROP_AFTER = 6 * 3600 * 1000      # an open session silent for 6 h is not shown
HISTORY = 30 * 24 * 3600 * 1000   # closed sessions are remembered for resuming
SLUG = re.compile(r'^[A-Za-z0-9._-]+$')
FID = re.compile(r'^[0-9a-f]{10}$')
SID = re.compile(r'^[0-9a-fA-F-]{8,64}$')
MAX_BODY = 16 * 1024


def child_env(claude):
    env = dict(os.environ)
    env['PATH'] = os.pathsep.join([os.path.dirname(claude), '/usr/local/bin', '/usr/bin', '/bin', env.get('PATH', '')])
    env.setdefault('HOME', os.path.expanduser('~'))
    return env


def active_sessions():
    """{sessionId: info} of sessions open right now, from `claude agents --json`; None if unavailable."""
    claude = common.claude_bin()
    if not claude:
        return None
    try:
        r = subprocess.run([claude, 'agents', '--json'], capture_output=True, text=True, timeout=15,
                           stdin=subprocess.DEVNULL, env=child_env(claude))
        data = json.loads(r.stdout)
        return {s['sessionId']: s for s in data if isinstance(s, dict) and s.get('sessionId')}
    except Exception:
        return None


def read_sessions(feats, now, include_closed=False):
    aliases = config().get('aliases', {})
    bound = common.bindings()
    out = []
    d = common.sessions_dir()
    if not os.path.isdir(d):
        return out
    for name in os.listdir(d):
        if not name.endswith('.json'):
            continue
        s = common.read_json(os.path.join(d, name))
        if not s:
            continue
        age = now - int(s.get('t', 0))
        state = s.get('state', 'idle')
        if state == 'closed':
            if not include_closed or age > HISTORY:
                continue
        elif age > DROP_AFTER:
            continue
        if state == 'work' and age > WORK_STALE:
            state = 'idle'
        project = aliases.get(s.get('project'), s.get('project'))
        out.append({'id': s.get('session', name[:-5]), 'project': project, 'branch': s.get('branch', ''),
                    'cwd': s.get('cwd', ''), 'state': state, 'what': s.get('what', ''), 't': s.get('t', now),
                    'feature': bound.get(s.get('session', name[:-5]))})
    out.sort(key=lambda x: -x['t'])
    return out


def read_events(feats, now, limit=30):
    aliases = config().get('aliases', {})
    bound = common.bindings()
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
        slug = bound.get(e.get('session'))
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


# ------------------------------------------------------------------ inbox and routing

PUBLIC = ('id', 'feature', 'fid', 'action', 'text', 'item', 'stage', 'state', 'route', 'session',
          'sessionName', 'ts', 'deliveredAt', 'note', 'error')


def public(e):
    return {k: e[k] for k in PUBLIC if k in e}


def plan_route(slug):
    """Decide where a message for this feature goes. No side effects.
    Returns (route, session_dict_or_None, session_name)."""
    feats = common.feature_index()
    now = int(time.time() * 1000)
    mine = [s for s in read_sessions(feats, now, include_closed=True) if s['feature'] == slug]
    agents = active_sessions()
    if agents is not None:
        live = [s for s in mine if s['id'] in agents and s['state'] != 'closed']
    else:                                   # claude not available: trust the hook data
        live = [s for s in mine if s['state'] != 'closed']
    if live:
        return 'active', live[0], (agents or {}).get(live[0]['id'], {}).get('name', '')
    closed = [s for s in mine if s['state'] == 'closed' or (agents is not None and s['id'] not in agents)]
    for s in closed:
        if SID.match(s['id']) and os.path.isdir(s.get('cwd') or ''):
            return 'bg', s, ''
    return 'none', None, ''


def resume_in_background(session, entry):
    claude = common.claude_bin()
    if not claude:
        return False, 'claude not found: set claudeBin in config.json'
    try:
        r = subprocess.run([claude, '--resume', session['id'], '--bg', common.format_message(entry)],
                           cwd=session['cwd'], capture_output=True, text=True, timeout=60,
                           stdin=subprocess.DEVNULL, env=child_env(claude))
    except Exception as e:
        return False, 'could not start claude: %s' % e
    out = ((r.stdout or '') + (r.stderr or '')).strip()
    return r.returncode == 0, out[:400]


def pick_list(slug):
    """Sessions that can be attached to a feature: every session the hook has seen recently,
    open ones first, with the name Claude Code gives it. Sessions already on this feature are left out."""
    now = int(time.time() * 1000)
    agents = active_sessions() or {}
    feats = common.feature_index()
    titles = {f['id']: f['title'] for f in feats}
    out, mine = [], []
    for s in read_sessions(feats, now, include_closed=True):
        live = s['id'] in agents and s['state'] != 'closed'
        row = {'id': s['id'], 'name': (agents.get(s['id']) or {}).get('name', ''), 'project': s['project'],
               'what': s['what'], 't': s['t'], 'open': live, 'state': s['state'] if live else 'closed',
               'other': titles.get(s['feature'], s['feature'] or '')}
        (mine if s['feature'] == slug else out).append(row)
    out.sort(key=lambda x: (not x['open'], -x['t']))
    mine.sort(key=lambda x: (not x['open'], -x['t']))
    return {'bound': mine, 'sessions': out[:40]}


def attach(body):
    slug, sid = body.get('feature'), body.get('session')
    if not (isinstance(slug, str) and SLUG.match(slug) and os.path.isfile(os.path.join(features_dir(), slug, 'feature.json'))):
        return 404, {'error': 'unknown feature'}
    if not (isinstance(sid, str) and SID.match(sid)):
        return 400, {'error': 'bad session'}
    common.bind(sid, slug)
    return 200, {'ok': True}


def act(body):
    if body.get('action') == 'attach':
        return attach(body)
    slug, fid, action = body.get('feature'), body.get('fid'), body.get('action')
    text = (body.get('text') or '').strip()
    if not (isinstance(slug, str) and SLUG.match(slug)):
        return 400, {'error': 'bad feature'}
    if not (isinstance(fid, str) and FID.match(fid)):
        return 400, {'error': 'bad item id'}
    if action not in ('do', 'ask', 'skip', 'unskip'):
        return 400, {'error': 'bad action'}
    if len(text) > 2000:
        return 400, {'error': 'text is too long (2000 characters max)'}
    src = os.path.join(features_dir(), slug, 'feature.json')
    if not os.path.isfile(src):
        return 404, {'error': 'unknown feature'}
    data = render.load(src)
    found = None
    for s in data['stages']:
        if fid in s['fids']:
            i = s['fids'].index(fid)
            found = (s, i, s['facts'][i])
    if not found:
        return 404, {'error': 'this item no longer exists, reload the page'}
    stage, i, fact = found

    if action in ('skip', 'unskip'):
        path = os.path.join(features_dir(), slug, 'overrides.json')
        ov = common.read_json(path)
        if action == 'skip':
            if fact[0] in ('y', 'fixed'):
                return 400, {'error': 'a finished item cannot be marked as not needed'}
            ov[fid] = {'kind': 'skip', 'reason': text, 'ts': int(time.time() * 1000)}
        else:
            ov.pop(fid, None)
        common.atomic_write_json(path, ov)
        return 200, {'ok': True, 'applied': action}

    entry = {'id': uuid.uuid4().hex[:12], 'ts': int(time.time() * 1000), 'state': 'new',
             'feature': slug, 'featureTitle': data['title'], 'fid': fid, 'stage': stage['n'],
             'stageTitle': stage['title'], 'item': fact[1], 'action': action, 'text': text}
    route, session, name = plan_route(slug)
    entry['route'] = route
    if route == 'active':
        entry.update(session=session['id'], sessionName=name)
    elif route == 'bg':
        ok, out = resume_in_background(session, entry)
        entry.update(session=session['id'])
        if ok:
            entry.update(state='delivered', deliveredAt=int(time.time() * 1000), note=out)
        else:
            entry.update(state='failed', error=out)
    common.save_entry(entry)
    return 200, {'ok': True, 'entry': public(entry)}


def inbox_state():
    now = int(time.time() * 1000)
    out = []
    for e in common.inbox_entries():
        if e.get('state') in ('done', 'failed') and now - e.get('ts', now) > 14 * 24 * 3600 * 1000:
            for ext in ('json', 'lock'):
                try:
                    os.remove(os.path.join(common.inbox_dir(), '%s.%s' % (e['id'], ext)))
                except OSError:
                    pass
            continue
        out.append(public(e))
    return out[-200:]


# ------------------------------------------------------------------ http

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

    def _json(self, code, obj):
        self._send(code, json.dumps(obj, ensure_ascii=False), 'application/json; charset=utf-8')

    def _hosts_ok(self):
        port = self.server.server_address[1]
        host = (self.headers.get('Host') or '')
        if host.split(':')[0] not in ('127.0.0.1', 'localhost'):
            return False
        origin = self.headers.get('Origin')
        return origin is None or origin in ('http://127.0.0.1:%d' % port, 'http://localhost:%d' % port)

    def do_GET(self):
        if not self._hosts_ok():
            return self._send(403, 'forbidden', 'text/plain; charset=utf-8')
        u = urlparse(self.path)
        path = u.path
        try:
            if path == '/':
                return self._send(200, render.index_html(state()), 'text/html; charset=utf-8')
            if path == '/api/state':
                return self._json(200, state())
            if path == '/api/inbox':
                return self._json(200, {'entries': inbox_state()})
            if path == '/api/sessions':
                slug = (parse_qs(u.query).get('feature') or [''])[0]
                if not SLUG.match(slug):
                    return self._json(400, {'error': 'bad feature'})
                return self._json(200, pick_list(slug))
            if path == '/api/route':
                slug = (parse_qs(u.query).get('feature') or [''])[0]
                if not SLUG.match(slug):
                    return self._json(400, {'error': 'bad feature'})
                route, session, name = plan_route(slug)
                return self._json(200, {'route': route, 'session': (session or {}).get('id'), 'name': name,
                                        'project': (session or {}).get('project', '')})
            m = re.match(r'^/f/([^/]+)/?$', path)
            if m and SLUG.match(m.group(1)):
                src = os.path.join(features_dir(), m.group(1), 'feature.json')
                if os.path.isfile(src):
                    html, _ = render.panel_html(src, back=True, back_href='/', live=True)
                    return self._send(200, html, 'text/html; charset=utf-8')
            return self._send(404, 'not found', 'text/plain; charset=utf-8')
        except render.DataError as e:
            return self._send(500, 'feature.json: %s' % e, 'text/plain; charset=utf-8')
        except Exception as e:  # keep the server alive whatever a data file contains
            return self._send(500, 'error: %s' % e, 'text/plain; charset=utf-8')

    def do_POST(self):
        # Anything on the web can make a browser POST to 127.0.0.1. Three checks stop that:
        # Host/Origin must be ours, the body must be JSON, and a custom header is required
        # (a cross-site page cannot add it without a CORS preflight, which this server never allows).
        if not self._hosts_ok():
            return self._send(403, 'forbidden', 'text/plain; charset=utf-8')
        if urlparse(self.path).path != '/api/act':
            return self._send(404, 'not found', 'text/plain; charset=utf-8')
        if not (self.headers.get('Content-Type') or '').startswith('application/json') or self.headers.get('X-Whatnow') != '1':
            return self._json(403, {'error': 'forbidden'})
        try:
            n = int(self.headers.get('Content-Length') or 0)
        except ValueError:
            n = 0
        if n <= 0 or n > MAX_BODY:
            return self._json(400, {'error': 'bad body size'})
        try:
            body = json.loads(self.rfile.read(n).decode('utf-8'))
            if not isinstance(body, dict):
                raise ValueError
        except ValueError:
            return self._json(400, {'error': 'bad json'})
        try:
            code, obj = act(body)
        except render.DataError as e:
            code, obj = 500, {'error': 'feature.json: %s' % e}
        except Exception as e:
            code, obj = 500, {'error': 'error: %s' % e}
        return self._json(code, obj)


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
