import http.client, json, os, subprocess, sys, tempfile, threading, time, unittest

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SKILL = os.path.join(ROOT, 'skill')
sys.path.insert(0, SKILL)

FEATURE = {
    'title': 'Demo feature', 'projects': ['api', 'web'], 'branches': [],
    'ladder': {'api': ['ok', 'ok', 'no', 'no', 'no', 'no'], 'web': ['ok', 'no', 'no', 'no', 'no', 'no']},
    'stages': [{'n': 1, 'title': 'Design', 'verdict': 'ok', 'plan': ['x'],
                'facts': [['y', 'Schema written'], ['q', 'Not done: check against the real database']]}],
}


class Env(unittest.TestCase):
    def setUp(self):
        self.home = tempfile.mkdtemp()
        self.feats = os.path.join(self.home, 'features')
        os.makedirs(os.path.join(self.feats, 'demo-feature'))
        with open(os.path.join(self.feats, 'demo-feature', 'feature.json'), 'w') as fh:
            json.dump(FEATURE, fh)
        self.saved = {k: os.environ.get(k) for k in ('WHATNOW_HOME', 'FEATURES_DIR')}
        os.environ['WHATNOW_HOME'] = self.home
        os.environ['FEATURES_DIR'] = self.feats

    def tearDown(self):
        for k, v in self.saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v

    def fid(self, idx=1):
        import render
        return render.load(os.path.join(self.feats, 'demo-feature', 'feature.json'))['stages'][0]['fids'][idx]

    def session(self, sid='11111111-aaaa', state='idle', branch='main', project='api', cwd=None, feature='demo-feature'):
        d = os.path.join(self.home, 'sessions')
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, sid + '.json'), 'w') as fh:
            json.dump({'session': sid, 'cwd': cwd or self.home, 'project': project, 'branch': branch,
                       'state': state, 'what': '', 't': int(time.time() * 1000)}, fh)
        if feature:
            import common
            common.bind(sid, feature)


class OverridesTest(Env):
    def test_skip_is_applied_and_reversible(self):
        import server
        code, obj = server.act({'feature': 'demo-feature', 'fid': self.fid(), 'action': 'skip', 'text': 'already checked'})
        self.assertEqual(code, 200)
        import render
        st = render.load(os.path.join(self.feats, 'demo-feature', 'feature.json'))['stages'][0]
        self.assertEqual(st['facts'][1][0], 'n')
        self.assertEqual(st['reasons']['1'], 'already checked')
        self.assertEqual(st['state'], 'ok')                       # nothing left undone
        # feature.json itself is never touched
        with open(os.path.join(self.feats, 'demo-feature', 'feature.json')) as fh:
            self.assertEqual(json.load(fh)['stages'][0]['facts'][1][0], 'q')
        server.act({'feature': 'demo-feature', 'fid': self.fid(), 'action': 'unskip'})
        st = render.load(os.path.join(self.feats, 'demo-feature', 'feature.json'))['stages'][0]
        self.assertEqual((st['facts'][1][0], st['state']), ('q', 'part'))

    def test_a_finished_item_cannot_be_skipped(self):
        import server
        code, _ = server.act({'feature': 'demo-feature', 'fid': self.fid(0), 'action': 'skip'})
        self.assertEqual(code, 400)

    def test_decision_follows_the_text_not_the_position(self):
        import server, render
        server.act({'feature': 'demo-feature', 'fid': self.fid(), 'action': 'skip', 'text': 'r'})
        path = os.path.join(self.feats, 'demo-feature', 'feature.json')
        with open(path) as fh:
            d = json.load(fh)
        d['stages'][0]['facts'].insert(0, ['y', 'A new first fact'])        # positions shift
        with open(path, 'w') as fh:
            json.dump(d, fh)
        st = render.load(path)['stages'][0]
        self.assertEqual([f[0] for f in st['facts']], ['y', 'y', 'n'])


class ValidationTest(Env):
    def test_rejects_bad_input(self):
        import server
        good = {'feature': 'demo-feature', 'fid': self.fid(), 'action': 'ask', 'text': 'why?'}
        for bad in ({'feature': '../etc'}, {'feature': 'nope-feature'}, {'fid': 'zzzz'}, {'fid': 'abcdef0123'},
                    {'action': 'rm -rf'}, {'text': 'x' * 2001}):
            code, _ = server.act(dict(good, **bad))
            self.assertIn(code, (400, 404), bad)


class RoutingTest(Env):
    def setUp(self):
        super().setUp()
        import server
        self.server = server
        self.launched = []
        self.saved_fns = (server.active_sessions, server.resume_in_background)
        server.resume_in_background = lambda s, e: (self.launched.append((s['id'], e['id'])) or True, 'started bg')

    def tearDown(self):
        self.server.active_sessions, self.server.resume_in_background = self.saved_fns
        super().tearDown()

    def send(self, action='do'):
        code, obj = self.server.act({'feature': 'demo-feature', 'fid': self.fid(), 'action': action, 'text': 'please'})
        self.assertEqual(code, 200)
        return obj['entry']

    def test_open_session_gets_it_addressed(self):
        self.session('11111111-aaaa', 'idle')
        self.server.active_sessions = lambda: {'11111111-aaaa': {'name': 'api-xy', 'status': 'idle'}}
        e = self.send()
        self.assertEqual((e['route'], e['state'], e['session'], e['sessionName']), ('active', 'new', '11111111-aaaa', 'api-xy'))
        self.assertEqual(self.launched, [])

    def test_closed_session_is_resumed_in_background_with_the_message(self):
        self.session('22222222-bbbb', 'closed')
        self.server.active_sessions = lambda: {}
        e = self.send()
        self.assertEqual((e['route'], e['state']), ('bg', 'delivered'))
        self.assertEqual(self.launched[0][0], '22222222-bbbb')

    def test_a_failed_resume_is_reported(self):
        self.session('22222222-bbbb', 'closed')
        self.server.active_sessions = lambda: {}
        self.server.resume_in_background = lambda s, e: (False, 'Workspace not trusted')
        e = self.send()
        self.assertEqual((e['state'], e['error']), ('failed', 'Workspace not trusted'))

    def test_no_known_session_waits(self):
        self.server.active_sessions = lambda: {}
        e = self.send('ask')
        self.assertEqual((e['route'], e['state']), ('none', 'new'))

    def test_session_of_another_feature_is_not_used(self):
        self.session('33333333-cccc', 'idle', feature=None)
        self.server.active_sessions = lambda: {'33333333-cccc': {'name': 'x'}}
        self.assertEqual(self.send()['route'], 'none')


class AttachTest(Env):
    def setUp(self):
        super().setUp()
        import server
        self.server = server
        server.active_sessions = lambda: {'44444444-dddd': {'name': 'api-work'}}

    def test_attach_links_a_session_and_the_picker_shows_it(self):
        self.session('44444444-dddd', 'idle', feature=None)
        self.session('55555555-eeee', 'closed', feature=None)
        pick = self.server.pick_list('demo-feature')
        self.assertEqual([s['id'] for s in pick['sessions']], ['44444444-dddd', '55555555-eeee'])   # open first
        self.assertEqual((pick['sessions'][0]['name'], pick['sessions'][0]['open']), ('api-work', True))
        self.assertEqual(pick['bound'], [])
        code, _ = self.server.act({'feature': 'demo-feature', 'action': 'attach', 'session': '44444444-dddd'})
        self.assertEqual(code, 200)
        pick = self.server.pick_list('demo-feature')
        self.assertEqual([s['id'] for s in pick['bound']], ['44444444-dddd'])
        self.assertEqual([s['id'] for s in pick['sessions']], ['55555555-eeee'])
        self.assertEqual(self.server.plan_route('demo-feature')[0], 'active')

    def test_attach_validates_input(self):
        self.assertEqual(self.server.act({'feature': 'nope', 'action': 'attach', 'session': '44444444-dddd'})[0], 404)
        self.assertEqual(self.server.act({'feature': 'demo-feature', 'action': 'attach', 'session': '../x'})[0], 400)

    def test_a_closed_attached_session_is_resumed_in_background(self):
        self.session('55555555-eeee', 'closed', feature=None)
        self.server.act({'feature': 'demo-feature', 'action': 'attach', 'session': '55555555-eeee'})
        self.assertEqual(self.server.plan_route('demo-feature')[0], 'bg')


class ClaimTest(Env):
    def entry(self, **kw):
        import common
        e = dict({'id': 'abc123def456', 'ts': 1, 'state': 'new', 'feature': 'demo-feature', 'featureTitle': 'Demo',
                  'stage': 1, 'stageTitle': 'Design', 'item': 'Not done', 'action': 'do', 'text': 'go'}, **kw)
        common.save_entry(e)
        return e

    def test_claim_is_exclusive_and_marks_delivered(self):
        import common
        self.entry(session='s1')
        self.assertEqual(len(common.claim('s2', 'demo-feature')), 0)          # addressed to someone else
        self.assertEqual(len(common.claim('s1', 'demo-feature')), 1)
        self.assertEqual(len(common.claim('s1', 'demo-feature')), 0)          # never twice
        self.assertEqual(common.read_json(os.path.join(common.inbox_dir(), 'abc123def456.json'))['state'], 'delivered')

    def test_unaddressed_message_goes_to_a_session_of_the_feature(self):
        import common
        self.entry()
        self.assertEqual(len(common.claim('s9', 'other-feature')), 0)
        self.assertEqual(len(common.claim('s9', 'demo-feature')), 1)

    def test_parallel_claims_deliver_once(self):
        import common
        self.entry()
        results = []
        ts = [threading.Thread(target=lambda: results.append(len(common.claim('s9', 'demo-feature')))) for _ in range(8)]
        [t.start() for t in ts]
        [t.join() for t in ts]
        self.assertEqual(sum(results), 1)


class HookDeliveryTest(Env):
    def run_hook(self, mode, payload, **env):
        e = dict(os.environ, WHATNOW_HOME=self.home, FEATURES_DIR=self.feats, **env)
        return subprocess.run([sys.executable, os.path.join(SKILL, 'hook.py'), mode], input=json.dumps(payload),
                              capture_output=True, text=True, env=e, timeout=30)

    def setUp(self):
        super().setUp()
        self.repo = os.path.join(tempfile.mkdtemp(), 'api')
        os.makedirs(self.repo)
        subprocess.run(['git', 'init', '-q', '-b', 'feature/demo-feature', self.repo], check=True)
        import common
        common.save_entry({'id': 'abc123def456', 'ts': 1, 'state': 'new', 'feature': 'demo-feature', 'featureTitle': 'Demo',
                           'stage': 1, 'stageTitle': 'Design', 'item': 'Not done', 'action': 'do', 'text': 'go'})
        common.bind('s1', 'demo-feature')
        self.payload = {'session_id': 's1', 'cwd': self.repo, 'hook_event_name': 'UserPromptSubmit'}

    def test_deliver_prints_the_message_once(self):
        r = self.run_hook('--deliver', self.payload)
        self.assertEqual(r.returncode, 0)
        self.assertIn('[whatnow] Message from the dashboard for feature "Demo"', r.stdout)
        self.assertIn('inbox.py resolve abc123def456', r.stdout)
        self.assertEqual(self.run_hook('--deliver', self.payload).stdout, '')

    def test_deliver_ignores_a_session_not_linked_to_the_feature(self):
        r = self.run_hook('--deliver', dict(self.payload, session_id='s2'))     # same repo and branch, but not linked
        self.assertEqual(r.stdout, '')

    def test_touching_the_feature_files_links_the_session(self):
        import common
        path = os.path.join(self.feats, 'demo-feature', 'feature.json')
        for sid, tool_input in (('e1', {'file_path': path}), ('e2', {'command': 'python3 render.py "%s" --open' % path}),
                                ('e3', {'command': 'ls %s/demo-feature' % self.feats})):
            self.run_hook('x', {'session_id': sid, 'cwd': self.repo, 'hook_event_name': 'PostToolUse',
                                'tool_name': 'Edit', 'tool_input': tool_input})
            self.assertEqual(common.feature_of(sid), 'demo-feature', sid)
        self.run_hook('x', {'session_id': 'e4', 'cwd': self.repo, 'hook_event_name': 'PostToolUse',
                            'tool_name': 'Edit', 'tool_input': {'file_path': '/tmp/other/feature.json'}})
        self.assertIsNone(common.feature_of('e4'))

    def test_taking_a_message_links_the_session(self):
        import common
        self.run_hook('--deliver', dict(self.payload, session_id='s9'))
        self.assertIsNone(common.feature_of('s9'))              # not linked: nothing is delivered to it
        self.assertEqual(common.inbox_entries()[0]['state'], 'new')
        common.bind('s9', 'demo-feature')
        self.run_hook('--deliver', dict(self.payload, session_id='s9'))
        self.assertEqual(common.inbox_entries()[0]['state'], 'delivered')

    def test_watcher_wakes_the_session_with_exit_code_2(self):
        r = self.run_hook('--watch', dict(self.payload, hook_event_name='Stop'), WHATNOW_WATCH_POLL='0.2')
        self.assertEqual(r.returncode, 2)
        self.assertIn('abc123def456', r.stderr)

    def test_watcher_exits_quietly_for_a_session_without_a_feature(self):
        r = self.run_hook('--watch', dict(self.payload, session_id='unlinked', hook_event_name='Stop'))
        self.assertEqual((r.returncode, r.stderr), (0, ''))

    def test_watcher_waits_then_gives_up_at_its_deadline(self):
        import common
        os.remove(os.path.join(common.inbox_dir(), 'abc123def456.json'))
        t = time.time()
        r = self.run_hook('--watch', dict(self.payload, hook_event_name='Stop'), WHATNOW_WATCH_POLL='0.2', WHATNOW_WATCH_SECONDS='1')
        self.assertEqual(r.returncode, 0)
        self.assertGreaterEqual(time.time() - t, 1)

    def test_watcher_picks_up_a_message_that_arrives_later(self):
        import common
        os.remove(os.path.join(common.inbox_dir(), 'abc123def456.json'))
        e = dict(os.environ, WHATNOW_HOME=self.home, FEATURES_DIR=self.feats, WHATNOW_WATCH_POLL='0.2')
        p = subprocess.Popen([sys.executable, os.path.join(SKILL, 'hook.py'), '--watch'], stdin=subprocess.PIPE,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=e)
        p.stdin.write(json.dumps(dict(self.payload, hook_event_name='Stop')))
        p.stdin.close()
        time.sleep(1.0)
        common.save_entry({'id': 'ffff00001111', 'ts': 2, 'state': 'new', 'feature': 'demo-feature', 'featureTitle': 'Demo',
                           'stage': 1, 'stageTitle': 'Design', 'item': 'X', 'action': 'ask', 'text': 'why?'})
        out, err = p.communicate(timeout=20)
        self.assertEqual(p.returncode, 2)
        self.assertIn('ffff00001111', err)


class InboxCliTest(Env):
    def test_resolve(self):
        import common
        common.save_entry({'id': 'abc123def456', 'ts': 1, 'state': 'delivered', 'feature': 'f', 'item': 'x', 'action': 'do'})
        env = dict(os.environ, WHATNOW_HOME=self.home)
        r = subprocess.run([sys.executable, os.path.join(SKILL, 'inbox.py'), 'resolve', 'abc123def456', 'done: checked'],
                           capture_output=True, text=True, env=env)
        self.assertEqual(r.returncode, 0)
        e = common.read_json(os.path.join(common.inbox_dir(), 'abc123def456.json'))
        self.assertEqual((e['state'], e['note']), ('done', 'done: checked'))
        bad = subprocess.run([sys.executable, os.path.join(SKILL, 'inbox.py'), 'resolve', '../../x'], capture_output=True, text=True, env=env)
        self.assertNotEqual(bad.returncode, 0)


class HttpSecurityTest(Env):
    def setUp(self):
        super().setUp()
        import server
        self.srv = server.ThreadingHTTPServer(('127.0.0.1', 0), server.Handler)
        self.port = self.srv.server_address[1]
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.server = server
        self.saved_fn = server.active_sessions
        server.active_sessions = lambda: {}

    def tearDown(self):
        self.srv.shutdown()
        self.srv.server_close()
        self.server.active_sessions = self.saved_fn
        super().tearDown()

    def req(self, method, path, body=None, headers=None):
        c = http.client.HTTPConnection('127.0.0.1', self.port, timeout=10)
        c.request(method, path, body=body, headers=headers or {})
        r = c.getresponse()
        data = r.read().decode()
        c.close()
        return r.status, data

    def post(self, headers=None, body=None):
        h = {'Content-Type': 'application/json', 'X-Whatnow': '1'}
        h.update(headers or {})
        b = json.dumps(body if body is not None else {'feature': 'demo-feature', 'fid': self.fid(), 'action': 'skip', 'text': 'r'})
        return self.req('POST', '/api/act', b, {k: v for k, v in h.items() if v is not None})

    def test_valid_post_works(self):
        self.assertEqual(self.post()[0], 200)

    def test_cross_site_posts_are_refused(self):
        self.assertEqual(self.post({'Origin': 'https://evil.example'})[0], 403)
        self.assertEqual(self.post({'X-Whatnow': None})[0], 403)                      # no custom header
        self.assertEqual(self.post({'Content-Type': 'text/plain'})[0], 403)           # a "simple" cross-site request
        self.assertEqual(self.post({'Host': 'evil.example'})[0], 403)                 # DNS rebinding
        self.assertFalse(os.path.exists(os.path.join(self.feats, 'demo-feature', 'overrides.json')))

    def test_body_limits_and_garbage(self):
        self.assertEqual(self.req('POST', '/api/act', 'x' * 20000, {'Content-Type': 'application/json', 'X-Whatnow': '1'})[0], 400)
        self.assertEqual(self.req('POST', '/api/act', 'not json', {'Content-Type': 'application/json', 'X-Whatnow': '1'})[0], 400)
        self.assertEqual(self.req('POST', '/api/other', '{}', {'Content-Type': 'application/json', 'X-Whatnow': '1'})[0], 404)

    def test_pages_and_inbox(self):
        code, html = self.req('GET', '/f/demo-feature/')
        self.assertEqual(code, 200)
        self.assertIn('"live": true', html)
        self.assertEqual(self.req('GET', '/f/../etc/')[0], 404)
        self.assertEqual(self.req('GET', '/api/inbox')[0], 200)
        self.assertEqual(self.req('GET', '/api/route?feature=demo-feature')[0], 200)
        self.assertEqual(self.req('GET', '/api/route?feature=../x')[0], 400)


class HooksInstallTest(unittest.TestCase):
    def setUp(self):
        import hooks_install
        self.h = hooks_install

    def test_install_is_idempotent_and_keeps_other_hooks(self):
        base = {'theme': 'x', 'hooks': {'Stop': [{'hooks': [{'type': 'command', 'command': 'echo mine'}]}]}}
        once = self.h.install(json.loads(json.dumps(base)))
        twice = self.h.install(json.loads(json.dumps(once)))
        self.assertEqual(once, twice)
        self.assertEqual(once['theme'], 'x')
        stop = [g['hooks'][0]['command'] for g in once['hooks']['Stop']]
        self.assertIn('echo mine', stop)
        self.assertEqual(sum('whatnow/hook.py' in c for c in stop), 2)       # state hook + watcher

    def test_wake_and_deliver_entries(self):
        d = self.h.install({})
        watch = [g for g in d['hooks']['Stop'] if '--watch' in g['hooks'][0]['command']][0]['hooks'][0]
        self.assertTrue(watch['asyncRewake'])
        deliver = [g for g in d['hooks']['UserPromptSubmit'] if '--deliver' in g['hooks'][0]['command']][0]['hooks'][0]
        self.assertNotIn('async', deliver)                                    # must be synchronous to reach the prompt

    def test_upgrade_from_the_old_set_only_adds(self):
        old = {'hooks': {'Stop': [{'hooks': [{'type': 'command', 'command': 'python3 ~/.claude/skills/whatnow/hook.py', 'async': True}]}]}}
        new = self.h.install(json.loads(json.dumps(old)))
        self.assertEqual(len(new['hooks']['Stop']), 2)

    def test_uninstall_removes_only_ours(self):
        d = self.h.install({'hooks': {'Stop': [{'hooks': [{'type': 'command', 'command': 'echo mine'}]}]}})
        d = self.h.uninstall(d)
        self.assertEqual(d, {'hooks': {'Stop': [{'hooks': [{'type': 'command', 'command': 'echo mine'}]}]}})


if __name__ == '__main__':
    unittest.main()
