import copy, json, os, re, shutil, subprocess, sys, tempfile, threading, time, unittest

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SKILL = os.path.join(ROOT, 'skill')
sys.path.insert(0, SKILL)

import hook, hooks_install, render  # noqa: E402


def example():
    with open(os.path.join(ROOT, 'examples', 'csv-export.json'), encoding='utf-8') as fh:
        return json.load(fh)


class RenderTest(unittest.TestCase):
    def root_with(self, data):
        root = tempfile.mkdtemp()
        os.mkdir(os.path.join(root, 'a'))
        with open(os.path.join(root, 'a', 'feature.json'), 'w', encoding='utf-8') as fh:
            fh.write(data if isinstance(data, str) else json.dumps(data))
        return root

    def test_planned_stage_without_facts_is_not_done(self):
        self.assertEqual(render.stage_state([]), 'no')

    def test_a_malformed_feature_is_reported_not_raised(self):
        def kind(d): d['stages'][0]['kind'] = 'party'
        def projects(d): d['projects'] = []
        def stage(d): d['stages'][0] = 'oops'
        def fact(d): d['stages'][0]['facts'] = [5]
        for change in (kind, projects, stage, fact):
            d = example()
            change(d)
            payload = render.collect(self.root_with(d))
            self.assertEqual((payload['features'], [b['id'] for b in payload['broken']]), ([], ['a']), change.__name__)
        for text in ('{not json', '[1, 2]'):
            payload = render.collect(self.root_with(text))
            self.assertEqual([b['id'] for b in payload['broken']], ['a'], text)

    def test_one_broken_feature_does_not_hide_the_others(self):
        bad = example()
        bad['projects'] = []
        root = self.root_with(bad)
        os.mkdir(os.path.join(root, 'b'))
        with open(os.path.join(root, 'b', 'feature.json'), 'w', encoding='utf-8') as fh:
            json.dump(example(), fh)
        payload = render.collect(root)
        self.assertEqual(([f['id'] for f in payload['features']], [b['id'] for b in payload['broken']]), (['b'], ['a']))

    def test_data_cannot_break_out_of_the_script_element(self):
        d = example()
        d['summary'] = '</script><script>alert(1)</script><!--<script>'
        root = self.root_with(d)
        html, _ = render.panel_html(os.path.join(root, 'a', 'feature.json'))
        data_script = re.search(r'<script>window\.PANEL=(.*?)</script>', html, re.S).group(1)
        self.assertNotIn('<', data_script)
        self.assertEqual(json.loads(data_script.rstrip().rstrip(';'))['summary'], d['summary'])
        self.assertNotIn('<', render.blob({'broken': [{'error': '</script>'}]}))


@unittest.skipUnless(shutil.which('node'), 'node is not installed')
class TemplateEscapeTest(unittest.TestCase):
    """rich() is what every text of feature.json goes through before it becomes HTML."""

    def rich(self, template, text):
        with open(os.path.join(SKILL, template), encoding='utf-8') as fh:
            src = fh.read()
        fn = re.search(r'^function rich\(t\)\{.*\}$', src, re.M).group(0)
        r = subprocess.run(['node', '-e', fn + ';process.stdout.write(rich(JSON.parse(process.argv[1])))', json.dumps(text)],
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r.stdout

    def test_only_code_and_b_survive(self):
        for template in ('template.html', 'template-index.html'):
            self.assertEqual(self.rich(template, 'run <code>make</code> <b>now</b>'), 'run <code>make</code> <b>now</b>')
            self.assertEqual(self.rich(template, '<img src=x onerror=alert(1)>'), '&lt;img src=x onerror=alert(1)&gt;')
            self.assertEqual(self.rich(template, '<b onclick="x()">t</b>'), '&lt;b onclick=&quot;x()&quot;&gt;t</b>')
            self.assertEqual(self.rich(template, 'Map<String, Int> & co'), 'Map&lt;String, Int&gt; &amp; co')
            self.assertEqual(self.rich(template, 'a &lt; b &amp; c'), 'a &lt; b &amp; c')

    def test_every_field_of_the_feature_page_is_escaped(self):
        with open(os.path.join(SKILL, 'template.html'), encoding='utf-8') as fh:
            src = fh.read()
        raw = re.findall(r"'\+(?:[a-zA-Z]+\.(?:t|title|verdict|sub|summary|name|hint|why|todo)|f\[1\]|x|p)\+'", src)
        self.assertEqual(raw, [])


class HookClassifyTest(unittest.TestCase):
    def run_bash(self, cmd, out='', failed=False):
        return hook.classify({'tool_input': {'command': cmd}, 'tool_response': {'stdout': out}}, failed)

    def test_a_pipe_does_not_hide_failed_tests(self):
        self.assertEqual(self.run_bash('pytest -q | tail -5', '=== 3 failed, 10 passed in 1.2s ==='),
                         ('tests', False, 'tests failed: 3 of 13'))
        self.assertEqual(self.run_bash('npx jest | tail', 'Tests:       1 failed, 5 passed, 6 total'),
                         ('tests', False, 'tests failed: 1 of 6'))
        self.assertEqual(self.run_bash('pytest', '=== 2 errors in 0.31s ==='), ('tests', False, 'tests failed: 2'))
        self.assertEqual(self.run_bash('pytest -q', '=== 10 passed in 1.2s ==='), ('tests', True, 'tests passed: 10'))

    def test_mentioning_a_test_tool_is_not_a_test_run(self):
        for cmd in ('pip install pytest', 'grep -rn unittest docs/', 'cat pytest.ini', 'npm install --save-dev jest',
                    'ls | grep vitest', 'echo "run pytest later"', 'uv add pytest'):
            self.assertEqual(self.run_bash(cmd), (None, None, ''), cmd)

    def test_real_runs_are_still_seen(self):
        for cmd in ('pytest', 'python3 -m pytest -q', 'cd api && CI=1 vendor/bin/phpunit', 'python3 -m unittest discover -s tests',
                    'npm test', 'npx vitest run', 'go test ./...', 'docker compose exec app phpunit', '(cd web && yarn test)'):
            self.assertEqual(self.run_bash(cmd)[0], 'tests', cmd)

    def test_only_our_own_watcher_is_ever_signalled(self):
        self.assertFalse(hook.is_watcher(os.getpid()))
        self.assertFalse(hook.is_watcher(2 ** 22 + 12345))


class HooksInstallTest(unittest.TestCase):
    def test_file_tools_reach_the_hook(self):
        # auto_bind links a session that edits feature.json; that needs the edit events, not only Bash
        matchers = [g.get('matcher', '') for g in hooks_install.desired()['PostToolUse']]
        for tool in ('Bash',) + hook.WRITE_TOOLS:
            self.assertTrue(any(re.fullmatch(m, tool) for m in matchers), tool)

    def test_the_example_in_docs_is_current(self):
        with open(os.path.join(ROOT, 'docs', 'hooks-settings.example.json'), encoding='utf-8') as fh:
            self.assertEqual(json.load(fh), {'hooks': hooks_install.desired()})


class AgentsCacheTest(unittest.TestCase):
    def test_parallel_requests_start_one_claude_agents(self):
        # a private copy of the module: other tests replace server.active_sessions with a stub
        import importlib.util
        spec = importlib.util.spec_from_file_location('server_for_cache_test', os.path.join(SKILL, 'server.py'))
        server = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(server)
        calls = []

        def slow():
            calls.append(1)
            time.sleep(0.3)
            return {'s': {}}

        server._active_sessions = slow
        got = []
        threads = [threading.Thread(target=lambda: got.append(server.active_sessions())) for _ in range(8)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.assertEqual((len(calls), got), (1, [{'s': {}}] * 8))
        server._AGENTS['t'] = time.time() - server.AGENTS_TTL - 1          # expired: one refresh, the rest get the old value
        threads = [threading.Thread(target=server.active_sessions) for _ in range(8)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.assertEqual(len(calls), 2)


if __name__ == '__main__':
    unittest.main()
