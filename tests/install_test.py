"""Installer contract tests with fake tools: no network or real installation."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="whatscli-installer-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.env = dict(os.environ, HOME=str(self.root),
                        PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                        WHATSCLI_DIR=str(self.root / "source with spaces"),
                        WHATSCLI_BIN_DIR=str(self.root / "local bin"))
        self.tool("git", """#!/bin/sh
set -eu
[ "${FAIL_CLONE:-}" != 1 ] || exit 1
for dest do :; done
mkdir -p "$dest"
printf 'module whatscli\\n' > "$dest/go.mod"
""")
        self.tool("go", """#!/bin/sh
set -eu
if [ "$1" = build ]; then
  [ "${FAIL_BUILD:-}" != 1 ] || exit 1
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then shift; out=$1; break; fi
    shift
  done
  printf '#!/bin/sh\\nprintf "test binary\\\\n"\\n' > "$out"
fi
""")

    def tool(self, name, source):
        path = self.tools / name
        path.write_text(source)
        path.chmod(0o755)

    def run_installer(self, *args, cwd=None):
        return subprocess.run(["sh", str(INSTALLER), *args],
                              env=self.env, cwd=cwd or self.root,
                              text=True, capture_output=True)

    def test_fresh_install_with_spaces(self):
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        binary = Path(self.env["WHATSCLI_DIR"]) / "bin/whatscli"
        link = Path(self.env["WHATSCLI_BIN_DIR"]) / "whatscli"
        self.assertTrue(os.access(binary, os.X_OK))
        self.assertEqual(link.resolve(), binary)
        self.assertFalse(list(self.root.glob(".whatscli-install.*")))

    def test_existing_checkout_is_never_overwritten(self):
        directory = Path(self.env["WHATSCLI_DIR"])
        directory.mkdir()
        marker = directory / "precious.txt"
        marker.write_text("unchanged")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(marker.read_text(), "unchanged")

    def test_existing_command_is_preserved(self):
        bindir = Path(self.env["WHATSCLI_BIN_DIR"])
        bindir.mkdir()
        command = bindir / "whatscli"
        command.write_text("existing program")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(command.read_text(), "existing program")

    def test_failed_build_leaves_no_checkout(self):
        self.env["FAIL_BUILD"] = "1"
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(Path(self.env["WHATSCLI_DIR"]).exists())
        self.assertFalse(list(self.root.glob(".whatscli-install.*")))

    def test_symlink_destination_rejected(self):
        destination = Path(self.env["WHATSCLI_DIR"])
        target = self.root / "target"
        target.mkdir()
        destination.symlink_to(target)
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(list(target.iterdir()))

    def test_unsupported_os_rejected(self):
        self.tool("uname", "#!/bin/sh\nprintf 'Darwin\\n'\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Linux", result.stderr)

    def test_local_rebuild_preserves_binary_on_failure(self):
        checkout = self.root / "checkout"
        checkout.mkdir()
        (checkout / "go.mod").write_text("module whatscli\n")
        (checkout / "bin").mkdir()
        binary = checkout / "bin/whatscli"
        binary.write_text("old binary")
        self.env["FAIL_BUILD"] = "1"
        result = self.run_installer("--local", cwd=checkout)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(binary.read_text(), "old binary")
        self.assertFalse(list((checkout / "bin").glob(".whatscli-build.*")))

    def test_local_rebuild(self):
        checkout = self.root / "checkout"
        checkout.mkdir()
        (checkout / "go.mod").write_text("module whatscli\n")
        result = self.run_installer("--local", cwd=checkout)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(os.access(checkout / "bin/whatscli", os.X_OK))


if __name__ == "__main__":
    unittest.main()
