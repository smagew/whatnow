import json, os, subprocess, sys, tempfile, unittest

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SKILL = os.path.join(ROOT, 'skill')
sys.path.insert(0, SKILL)


class HookTest(unittest.TestCase):
    def setUp(self):
        self.home = tempfile.mkdtemp()
        self.repo = tempfile.mkdtemp()
        subprocess.run(['git', 'init', '-q', '-b', 'feature/demo-feature', self.repo], check=True)

    def send(self, **payload):
        payload.setdefault('session_id', 's1')
        payload.setdefault('cwd', self.repo)
        env = dict(os.environ, WHATNOW_HOME=self.home)
        r = subprocess.run([sys.executable, os.path.join(SKILL, 'hook.py')], input=json.dumps(payload),
                           capture_output=True, text=True, env=env)
        self.assertEqual(r.returncode, 0)
        self.assertEqual(r.stdout + r.stderr, '')     # a hook must be silent
        return r

    def events(self):
        p = os.path.join(self.home, 'events.jsonl')
        if not os.path.exists(p):
            return []
        with open(p) as fh:
            return [json.loads(l) for l in fh]

    def session(self):
        with open(os.path.join(self.home, 'sessions', 's1.json')) as fh:
            return json.load(fh)

    def bash(self, cmd, ok=True, out=''):
        if ok:
            self.send(hook_event_name='PostToolUse', tool_name='Bash', tool_input={'command': cmd}, tool_response={'stdout': out})
        else:
            self.send(hook_event_name='PostToolUseFailure', tool_name='Bash', tool_input={'command': cmd}, error=out)

    def test_filter_keeps_only_git_and_tests(self):
        self.bash('ls -la')
        self.bash('git status')
        self.assertEqual(self.events(), [])
        self.bash('git commit -m "Add thing"')
        self.bash('git push -u origin x')
        self.bash('vendor/bin/phpunit', out='OK (214 tests, 900 assertions)')
        self.bash('vendor/bin/phpunit', ok=False, out='Exit code 1\nTests: 214, Assertions: 900, Failures: 2.')
        kinds = [(e['kind'], e['ok']) for e in self.events()]
        self.assertEqual(kinds, [('commit', True), ('push', True), ('tests', True), ('tests', False)])
        self.assertIn('Add thing', self.events()[0]['text'])
        self.assertEqual(self.events()[2]['text'], 'тесты прошли: 214')
        self.assertEqual(self.events()[3]['text'], 'тесты упали: 2 из 214')
        self.assertEqual(self.events()[0]['branch'], 'feature/demo-feature')

    def test_python_unittest_output_on_stderr(self):
        self.send(hook_event_name='PostToolUse', tool_name='Bash', tool_input={'command': 'python3 -m unittest discover -s tests'},
                  tool_response={'stdout': '', 'stderr': 'Ran 5 tests in 2.1s\n\nOK\n'})
        self.send(hook_event_name='PostToolUseFailure', tool_name='Bash', tool_input={'command': 'python3 -m unittest'},
                  error='Exit code 1\nRan 5 tests in 1s\n\nFAILED (failures=2)')
        self.assertEqual([e['text'] for e in self.events()], ['тесты прошли: 5', 'тесты упали: 2 из 5'])

    def test_state_machine(self):
        self.send(hook_event_name='UserPromptSubmit', prompt='secret')
        self.assertEqual(self.session()['state'], 'work')
        self.send(hook_event_name='Notification', notification_type='permission_prompt')
        self.assertEqual(self.session()['state'], 'wait')
        self.bash('echo hi')                              # work resumed after approval
        self.assertEqual(self.session()['state'], 'work')
        self.send(hook_event_name='Stop')
        self.assertEqual(self.session()['state'], 'wait')
        self.send(hook_event_name='Notification', notification_type='idle_prompt')
        self.assertEqual(self.session()['state'], 'idle')
        self.send(hook_event_name='SessionEnd')
        self.assertFalse(os.path.exists(os.path.join(self.home, 'sessions', 's1.json')))

    def test_prompt_text_is_never_stored(self):
        self.send(hook_event_name='UserPromptSubmit', prompt='very secret prompt')
        blob = ''
        for dp, _, fs in os.walk(self.home):
            for f in fs:
                with open(os.path.join(dp, f)) as fh:
                    blob += fh.read()
        self.assertNotIn('very secret prompt', blob)

    def test_never_fails_on_garbage(self):
        env = dict(os.environ, WHATNOW_HOME=self.home)
        for bad in ('', 'not json', '[]', '{"hook_event_name": 5}'):
            r = subprocess.run([sys.executable, os.path.join(SKILL, 'hook.py')], input=bad, capture_output=True, text=True, env=env)
            self.assertEqual((r.returncode, r.stdout, r.stderr), (0, '', ''))


class ServerMatchTest(unittest.TestCase):
    def test_branch_to_feature(self):
        import server
        feats = [{'id': 'csv-export', 'projects': ['api', 'web'], 'branches': []},
                 {'id': 'reports', 'projects': ['api'], 'branches': ['hotfix/export']}]
        m = server.match_feature
        self.assertEqual(m(feats, 'api', 'feature/csv-export'), 'csv-export')
        self.assertIsNone(m(feats, 'mobile', 'feature/csv-export'))      # project not part of the feature
        self.assertEqual(m(feats, 'api', 'hotfix/export'), 'reports')          # explicit "branches"
        self.assertIsNone(m(feats, 'api', 'master'))


if __name__ == '__main__':
    unittest.main()
