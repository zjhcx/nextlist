"""Check prepared-frontend routing without network access or cross compilers."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class PreparedFrontendTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        shutil.copy(Path(__file__).resolve().parents[2] / 'build.sh', self.root)
        self.source = self.root / 'web/dist'
        self.source.mkdir(parents=True)
        (self.source / 'index.html').write_text('<html>local frontend</html>')
        (self.source / 'VERSION').write_text('v1.2.3-local\n')
        self.destination = self.root / 'public/dist'
        self.destination.mkdir(parents=True)
        (self.destination / 'stale.js').write_text('old build')
        (self.destination / 'README.md').write_text('keep')
        stubs = self.root / 'stubs'
        stubs.mkdir()
        scripts = {
            'git': 'printf "%s\\n" "$*" >> "$GIT_ARGS_LOG"\ncase "$1" in log) echo deadbee;; describe) echo v1.2.3;; *) exit 99;; esac',
            'curl': 'echo unexpected-network >> "$NETWORK_LOG"\nexit 99',
            'go': 'printf "%s\\n" "$@" > "$GO_ARGS_LOG"',
        }
        for name, body in scripts.items():
            p = stubs / name
            p.write_text('#!/bin/bash\n' + body + '\n')
            p.chmod(0o755)
        self.env = dict(os.environ, PATH=str(stubs) + os.pathsep + os.environ['PATH'],
                        OPENLIST_FRONTEND_DIR='web/dist', OPENLIST_VERSION='v1.2.3-custom',
                        GITHUB_TOKEN='', GO_ARGS_LOG=str(self.root / 'go.log'),
                        GIT_ARGS_LOG=str(self.root / 'git.log'), NETWORK_LOG=str(self.root / 'network.log'))

    def run_build(self, *args, success=True):
        result = subprocess.run(['bash', 'build.sh', *args], cwd=self.root,
                                env=self.env, capture_output=True, text=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / 'network.log').exists(), 'contacted upstream frontend')
        self.assertNotIn('tag -d', (self.root / 'git.log').read_text())
        return result

    def test_build_modes_use_prepared_frontend(self):
        for mode in ['dev', 'beta', 'release']:
            with self.subTest(mode=mode):
                self.run_build(mode, 'web')
                self.assertEqual((self.destination / 'index.html').read_bytes(), (self.source / 'index.html').read_bytes())
                self.assertEqual((self.destination / 'VERSION').read_text().strip(), 'v1.2.3-local')
                self.assertFalse((self.destination / 'stale.js').exists())
                self.assertTrue((self.destination / 'README.md').exists())

    def test_backend_receives_both_versions(self):
        self.run_build('release', 'docker')
        flags = (self.root / 'go.log').read_text()
        self.assertIn('conf.Version=v1.2.3-custom', flags)
        self.assertIn('conf.WebVersion=v1.2.3-local', flags)

    def test_lite_accepts_prepared_variant(self):
        self.run_build('release', 'lite', 'web')

    def test_already_staged_frontend_is_preserved(self):
        shutil.copytree(self.source, self.destination, dirs_exist_ok=True)
        self.env['OPENLIST_FRONTEND_DIR'] = 'public/dist'
        self.run_build('release', 'web')
        self.assertTrue((self.destination / 'index.html').exists())

    def test_missing_index_fails_without_download(self):
        (self.source / 'index.html').unlink()
        self.run_build('release', 'web', success=False)

    def test_missing_version_fails_without_download(self):
        (self.source / 'VERSION').unlink()
        self.run_build('release', 'web', success=False)

    def test_empty_version_fails_without_download(self):
        (self.source / 'VERSION').write_text('\n')
        self.run_build('release', 'web', success=False)

    def test_nested_source_is_rejected_without_deleting_it(self):
        nested = self.destination / 'nested'
        shutil.copytree(self.source, nested)
        self.env['OPENLIST_FRONTEND_DIR'] = str(nested)
        self.run_build('release', 'web', success=False)
        self.assertTrue((nested / 'index.html').exists())


if __name__ == '__main__':
    unittest.main()
