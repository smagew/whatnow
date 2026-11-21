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
        self.assertEqual(self.events()[2]['text'], 'tests passed: 214')
        self.assertEqual(self.events()[3]['text'], 'tests failed: 2 of 214')
        self.assertEqual(self.events()[0]['branch'], 'feature/demo-feature')

    def test_python_unittest_output_on_stderr(self):
        self.send(hook_event_name='PostToolUse', tool_name='Bash', tool_input={'command': 'python3 -m unittest discover -s tests'},
                  tool_response={'stdout': '', 'stderr': 'Ran 5 tests in 2.1s\n\nOK\n'})
        self.send(hook_event_name='PostToolUseFailure', tool_name='Bash', tool_input={'command': 'python3 -m unittest'},
                  error='Exit code 1\nRan 5 tests in 1s\n\nFAILED (failures=2)')
        self.assertEqual([e['text'] for e in self.events()], ['tests passed: 5', 'tests failed: 2 of 5'])

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
        self.assertEqual(self.session()['state'], 'closed')        # kept: the page can resume it later

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


class BindingTest(unittest.TestCase):
    def test_bind_and_find(self):
        import common
        old = os.environ.get('WHATNOW_HOME')
        os.environ['WHATNOW_HOME'] = tempfile.mkdtemp()
        try:
            self.assertIsNone(common.feature_of('s1'))
            self.assertTrue(common.bind('s1', 'csv-export'))
            self.assertFalse(common.bind('s1', 'csv-export'))           # nothing changed
            self.assertTrue(common.bind('s1', 'reports'))               # the last feature wins
            self.assertEqual(common.feature_of('s1'), 'reports')
            self.assertFalse(common.bind('', 'x'))
        finally:
            if old is None:
                os.environ.pop('WHATNOW_HOME')
            else:
                os.environ['WHATNOW_HOME'] = old


class ConfigTest(unittest.TestCase):
    def run_in(self, env, code):
        e = {k: v for k, v in os.environ.items() if k not in ('WHATNOW_HOME', 'FEATURES_DIR')}
        e.update(env)
        r = subprocess.run([sys.executable, '-c', 'import sys; sys.path.insert(0, %r); import common; %s' % (SKILL, code)],
                           capture_output=True, text=True, env=e)
        return r.stdout.strip()

    def test_defaults_do_not_depend_on_the_machine(self):
        home = tempfile.mkdtemp()
        self.assertEqual(self.run_in({'WHATNOW_HOME': home}, 'print(common.features_dir())'), os.path.realpath(home) + '/features'
                         if os.path.realpath(home) == home else os.path.join(home, 'features'))
        self.assertEqual(self.run_in({'WHATNOW_HOME': home}, 'print(common.port())'), '7777')

    def test_config_file_and_env_override(self):
        home = tempfile.mkdtemp()
        with open(os.path.join(home, 'config.json'), 'w') as fh:
            json.dump({'featuresDir': '/data/feats', 'port': 8123}, fh)
        self.assertEqual(self.run_in({'WHATNOW_HOME': home}, 'print(common.features_dir())'), '/data/feats')
        self.assertEqual(self.run_in({'WHATNOW_HOME': home}, 'print(common.port())'), '8123')
        self.assertEqual(self.run_in({'WHATNOW_HOME': home, 'FEATURES_DIR': '/other'}, 'print(common.features_dir())'), '/other')

    def test_broken_config_falls_back_to_defaults(self):
        home = tempfile.mkdtemp()
        with open(os.path.join(home, 'config.json'), 'w') as fh:
            fh.write('{broken')
        self.assertEqual(self.run_in({'WHATNOW_HOME': home}, 'print(common.port())'), '7777')


class AutostartTest(unittest.TestCase):
    def test_plist_is_valid_and_portable(self):
        import plistlib, autostart
        text = autostart.build_plist(['/py/python3', '/s/server.py', '--port', '7777'], '/h/server.log', {'WHATNOW_HOME': '/h'})
        d = plistlib.loads(text.encode())
        self.assertEqual(d['Label'], 'whatnow.server')
        self.assertTrue(d['RunAtLoad'] and d['KeepAlive'])
        self.assertEqual(d['ProgramArguments'], ['/py/python3', '/s/server.py', '--port', '7777'])
        self.assertEqual(d['EnvironmentVariables'], {'WHATNOW_HOME': '/h'})

    def test_systemd_unit(self):
        import autostart
        unit = autostart.build_unit(['/py/python3', '/path with space/server.py', '--port', '7777'], '/h/server.log', {})
        self.assertIn('ExecStart="/py/python3" "/path with space/server.py" "--port" "7777"', unit)
        self.assertIn('Restart=on-failure', unit)
        self.assertIn('WantedBy=default.target', unit)
        self.assertNotIn('Environment=', unit)

    def test_no_machine_specific_path_in_the_source(self):
        for name in ('autostart.py', 'common.py', 'render.py', 'server.py', 'hook.py', 'hooks_install.py'):
            with open(os.path.join(SKILL, name), encoding='utf-8') as fh:
                src = fh.read()
            self.assertNotIn('/Users/', src, name)
            self.assertNotIn('/www/', src, name)


class FixedFactTest(unittest.TestCase):
    def test_fixed_does_not_colour_the_stage(self):
        import render
        self.assertEqual(render.stage_state([['y', 'a'], ['fixed', 'b']]), 'ok')
        self.assertEqual(render.stage_state([['fixed', 'b'], ['q', 'c']]), 'part')
        self.assertEqual(render.stage_state([['fixed', 'b'], ['bad', 'open']]), 'bad')
        self.assertEqual(render.stage_state([['q', 'a'], ['q', 'b']]), 'no')

    def test_stage_colour_says_how_worried_to_be(self):
        import render
        nine = [['y', str(i)] for i in range(9)]
        self.assertEqual(render.stage_state(nine + [['q', 'x']]), 'almost')       # 90% done
        self.assertEqual(render.stage_state(nine[:4] + [['q', 'x']]), 'almost')   # 80% done
        self.assertEqual(render.stage_state(nine[:3] + [['q', 'x']]), 'part')     # 75% done
        self.assertEqual(render.stage_state(nine + [['bad', 'x']]), 'bad')        # an open problem always shows


class ProgressTest(unittest.TestCase):
    """The Progress tab: the item in work, the current stage and the change log."""

    def feature(self, stages):
        return {'title': 't', 'projects': ['a'], 'ladder': {'a': ['no'] * 6}, 'stages': stages}

    def stage(self, n, facts):
        return {'n': n, 'title': 's%d' % n, 'verdict': '', 'plan': [], 'facts': facts}

    def render(self, d, stamp):
        import render
        with open(self.src, 'w', encoding='utf-8') as fh:
            json.dump(d, fh)
        os.utime(self.src, (stamp, stamp))
        _, data = render.panel_html(self.src)
        return data

    def setUp(self):
        self.src = os.path.join(tempfile.mkdtemp(), 'feature.json')

    def test_item_in_work_is_not_done_and_marks_the_current_stage(self):
        import render
        self.assertEqual(render.stage_state([['w', 'a'], ['q', 'b']]), 'part')
        d = render.prepare(self.feature([self.stage(1, [['y', 'a']]), self.stage(2, [['w', 'b'], ['q', 'c']]),
                                         self.stage(3, [['y', 'd']]), self.stage(4, [])]))
        self.assertEqual(d['cur'], 2)                    # the item in work wins over a later started stage
        self.assertEqual((d['stages'][1]['dn'], d['stages'][1]['tot']), (0, 2))

    def test_current_stage_without_an_item_in_work(self):
        import render
        d = render.prepare(self.feature([self.stage(1, [['y', 'a']]), self.stage(2, [['p', 'b']]), self.stage(3, [])]))
        self.assertEqual(d['cur'], 2)                    # the furthest stage that was started
        self.assertEqual(render.prepare(self.feature([self.stage(1, []), self.stage(2, [])]))['cur'], 1)

    def test_deploy_item_cannot_be_in_work(self):
        import render
        d = self.feature([self.stage(1, [])])
        d['deploy'] = [{'n': 1, 'title': 'x', 'sub': '', 'items': [{'t': 'a', 's': 'w'}]}]
        self.assertRaises(render.DataError, render.prepare, d)

    def test_change_log(self):
        first = self.render(self.feature([self.stage(1, [['q', 'a'], ['q', 'b'], ['y', 'c']])]), 1000)
        self.assertEqual(first['changes'], [])           # the first render only takes a snapshot
        self.assertEqual(first['updatedTs'], 1000000)
        # a -> in work; b rewritten in place and finished; c untouched; d is new
        data = self.render(self.feature([self.stage(1, [['w', 'a'], ['y', 'b, done'], ['y', 'c'], ['bad', 'd']])]), 2000)
        got = [(c['was'], c['now'], c['t'], c['ts']) for c in data['changes']]
        self.assertEqual(got, [('q', 'w', 'a', 2000000), ('q', 'y', 'b, done', 2000000), (None, 'bad', 'd', 2000000)])
        ids = dict(zip(['a', 'b', 'c', 'd'], data['stages'][0]['fids']))
        self.assertEqual([c['fid'] for c in data['changes']], [ids['a'], ids['b'], ids['d']])
        # rendering again without a change adds nothing; a rewrite with the same status keeps the entry on the item
        self.assertEqual(len(self.render(self.feature([self.stage(1, [['w', 'a'], ['y', 'b, done'], ['y', 'c'], ['bad', 'd']])]), 2000)['changes']), 3)
        data = self.render(self.feature([self.stage(1, [['w', 'a'], ['y', 'b, done and checked'], ['y', 'c'], ['bad', 'd']])]), 3000)
        self.assertEqual(len(data['changes']), 3)
        self.assertEqual(data['changes'][1]['fid'], data['stages'][0]['fids'][1])

    def test_a_file_that_is_not_a_feature_keeps_no_history(self):
        import render
        self.src = os.path.join(os.path.dirname(self.src), 'example.json')
        self.render(self.feature([self.stage(1, [['q', 'a']])]), 1000)
        self.assertFalse(os.path.exists(os.path.join(os.path.dirname(self.src), 'history.json')))


if __name__ == '__main__':
    unittest.main()
