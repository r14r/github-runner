"""Regression tests for new multi-repository runner workflow management."""
from pathlib import Path
import importlib.util
import tempfile
import unittest

PATCHER = Path(__file__).resolve().parents[1] / 'runner/lib/patch_workflow.py'
spec = importlib.util.spec_from_file_location('runner_workflow_patch', PATCHER)
module = importlib.util.module_from_spec(spec)
import sys
sys.modules[spec.name] = module
spec.loader.exec_module(module)
patch_file = module.patch_file
patch_files = module.patch_files


class WorkflowPatchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name) / '.github/workflows'
        self.base.mkdir(parents=True)
        self.file = self.base / 'ci.yml'

    def write(self, content):
        self.file.write_text(content)
        return self.file

    def test_linux_runner_works_with_ubuntu(self):
        self.write('jobs:\n  lint:\n    runs-on: ubuntu-latest\n  test:\n    runs-on: ubuntu-latest\n')
        self.assertEqual(patch_file(self.file, 'repo-ci', False, {'lint'}, 'Linux'), 1)
        self.assertIn('    runs-on: ubuntu-latest', self.file.read_text())
        self.assertIn('  lint:\n    runs-on: [self-hosted, Linux, X64, repo-ci]', self.file.read_text())

    def test_idempotent(self):
        self.write('jobs:\n  lint:\n    runs-on: ubuntu-latest\n')
        self.assertEqual(patch_file(self.file, 'repo-ci', False, os_name='Linux'), 1)
        self.assertEqual(patch_file(self.file, 'repo-ci', False, os_name='Linux'), 0)

    def test_missing_job_refuses_all_writes(self):
        original = 'jobs:\n  lint:\n    runs-on: ubuntu-latest\n'
        self.write(original)
        with self.assertRaisesRegex(ValueError, 'missing'):
            patch_file(self.file, 'ci', False, {'lint', 'missing'}, 'Linux')
        self.assertEqual(self.file.read_text(), original)

    def test_dynamic_unsupported(self):
        original = 'jobs:\n  lint:\n    runs-on: ${{ matrix.os }}\n'
        self.write(original)
        with self.assertRaisesRegex(ValueError, 'dynamic'):
            patch_file(self.file, 'ci', False, os_name='Linux')
        self.assertEqual(self.file.read_text(), original)

    def test_multi_file_validate_before_write(self):
        a = self.write('jobs:\n  lint:\n    runs-on: ubuntu-latest\n')
        b = self.base / 'other.yml'
        b.write_text('jobs:\n  test:\n    runs-on: ${{ matrix.os }}\n')
        with self.assertRaisesRegex(ValueError, 'dynamic'):
            patch_files([a,b], 'ci', False, os_name='Linux')
        self.assertEqual(a.read_text(), 'jobs:\n  lint:\n    runs-on: ubuntu-latest\n')

    def test_jobs_across_files(self):
        a = self.write('jobs:\n  lint:\n    runs-on: ubuntu-latest\n')
        b = self.base / 'other.yml'
        b.write_text('jobs:\n  test:\n    runs-on: ubuntu-latest\n')
        self.assertEqual(patch_files([a,b], 'ci', False, {'lint', 'test'}, 'Linux'), 2)

    def test_cross_os_rejected(self):
        self.write('jobs:\n  lint:\n    runs-on: macos-latest\n')
        with self.assertRaisesRegex(ValueError, 'allow-cross-os'):
            patch_file(self.file, 'ci', False, os_name='Linux')

    def test_step_text_untouched(self):
        self.write("jobs:\n  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo runs-on: ubuntu-latest\n")
        patch_file(self.file, 'ci', False, os_name='Linux')
        self.assertIn('echo runs-on: ubuntu-latest', self.file.read_text())

    def test_symlink_refused(self):
        target = self.write('jobs:\n  lint:\n    runs-on: ubuntu-latest\n')
        link = self.base / 'ref.yml'
        link.symlink_to(target)
        with self.assertRaisesRegex(ValueError, 'Unsafe'):
            patch_file(link, 'ci', False, os_name='Linux')


if __name__ == '__main__':
    unittest.main()
