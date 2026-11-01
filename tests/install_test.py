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
if [ "${1:-}" = "-C" ]; then
  shift 2
  case "$1" in
    remote) printf '%s\\n' "${FAKE_ORIGIN:-https://github.com/sepisotoni/WhatsCLI.git}" ;;
    status) [ "${DIRTY:-}" != 1 ] || printf ' M README.md\\n' ;;
    fetch) [ "${FAIL_FETCH:-}" != 1 ] ;;
    checkout) [ "${FAIL_CHECKOUT:-}" != 1 ] ;;
    *) exit 2 ;;
  esac
  exit
fi
[ "${FAIL_CLONE:-}" != 1 ] || exit 1
for dest do :; done
mkdir -p "$dest"
printf 'module whatscli\\n' > "$dest/go.mod"
printf '0.4.0\\n' > "$dest/VERSION"
mkdir -p "$dest/.git"
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
        self.tool("curl", """#!/bin/sh
set -eu
[ "${FAIL_RELEASE_LOOKUP:-}" != 1 ] || exit 1
case "$*" in
  *'releases?per_page=100'*)
    printf '[{"tag_name":"v0.5.0-beta.1","prerelease":true,"draft":false},{"tag_name":"v0.4.0","prerelease":false,"draft":false}]\\n'
    ;;
  *) printf '{"tag_name":"v0.4.0"}\\n' ;;
esac
""")

    def tool(self, name, source):
        path = self.tools / name
        path.write_text(source)
        path.chmod(0o755)

    def run_installer(self, *args, cwd=None):
        return subprocess.run(["sh", str(INSTALLER), *args],
                              env=self.env, cwd=cwd or self.root,
                              text=True, capture_output=True)

    def update_checkout(self):
        directory = self.root / f"checkout-{len(list(self.root.glob('checkout-*')))}"
        self.env["WHATSCLI_DIR"] = str(directory)
        directory.mkdir()
        (directory / ".git").mkdir()
        (directory / "go.mod").write_text("module whatscli\n")
        (directory / "VERSION").write_text("0.4.0\n")
        (directory / "bin").mkdir()
        binary = directory / "bin/whatscli"
        binary.write_text("old binary")
        return directory, binary

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
        (checkout / "VERSION").write_text("0.4.0\n")
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
        (checkout / "VERSION").write_text("0.4.0\n")
        result = self.run_installer("--local", cwd=checkout)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(os.access(checkout / "bin/whatscli", os.X_OK))

    def test_update_rejects_dirty_checkout(self):
        self.update_checkout()
        self.env["DIRTY"] = "1"
        result = self.run_installer("--update")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("local changes", result.stderr)

    def test_update_requires_expected_origin(self):
        self.update_checkout()
        self.env["FAKE_ORIGIN"] = "https://example.test/other.git"
        result = self.run_installer("--update")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(Path(self.env["WHATSCLI_DIR"], "bin/whatscli").read_text(), "old binary")

    def test_update_fetch_or_checkout_failure_preserves_binary(self):
        for variable in ("FAIL_FETCH", "FAIL_CHECKOUT"):
            with self.subTest(variable=variable):
                self.update_checkout()
                self.env[variable] = "1"
                result = self.run_installer("--update")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(Path(self.env["WHATSCLI_DIR"], "bin/whatscli").read_text(), "old binary")
                self.env.pop(variable)

    def test_update_build_failure_preserves_binary(self):
        directory, binary = self.update_checkout()
        self.env["FAIL_BUILD"] = "1"
        result = self.run_installer("--update")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(binary.read_text(), "old binary")
        self.assertFalse(list((directory / "bin").glob(".whatscli-build.*")))

    def test_update_rebuilds_binary_after_fast_forward(self):
        _, binary = self.update_checkout()
        result = self.run_installer("--update")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(binary.read_text(), "old binary")
        self.assertTrue(os.access(binary, os.X_OK))

    def test_curl_style_invocation_updates_existing_install(self):
        _, binary = self.update_checkout()
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(binary.read_text(), "old binary")
        self.assertTrue(os.access(binary, os.X_OK))

    def test_beta_flag_selects_latest_prerelease(self):
        _, binary = self.update_checkout()
        result = self.run_installer("--beta", "--update")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("v0.5.0-beta.1", result.stdout)
        self.assertNotEqual(binary.read_text(), "old binary")

    def test_invalid_release_ref_is_rejected(self):
        self.update_checkout()
        self.env["WHATSCLI_REF"] = "latest"
        result = self.run_installer("--update")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("stable/beta release tag", result.stderr)

    def test_latest_release_lookup_failure_does_not_replace_binary(self):
        _, binary = self.update_checkout()
        self.env["FAIL_RELEASE_LOOKUP"] = "1"
        result = self.run_installer("--update")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(binary.read_text(), "old binary")


if __name__ == "__main__":
    unittest.main()
