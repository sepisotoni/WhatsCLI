"""Linux PTY acceptance checks that mimic an SSH session (no COLORTERM, a plain
xterm-256color TERM, resizes, SGR mouse reports, and a dropped connection).
Build the binary first: python3 tests/terminal_smoke.py ./whats-cli"""
import fcntl
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time


class Session:
    def __init__(self, binary, args, rows=42, cols=130, extra_env=None):
        self.tmp = tempfile.TemporaryDirectory(prefix="whatscli-pty-")
        self.root = Path(self.tmp.name)
        (self.root / "tmp").mkdir()
        self.master, slave = pty.openpty()
        self.resize(rows, cols)
        env = {k: v for k, v in os.environ.items() if k not in ("COLORTERM", "TERM_PROGRAM")}
        env.update(TERM="xterm-256color", XDG_CONFIG_HOME=str(self.root / "config"),
                   TMPDIR=str(self.root / "tmp"), LANG="C.UTF-8")
        env.update(extra_env or {})
        self.proc = subprocess.Popen([binary, *args], stdin=slave, stdout=slave, stderr=slave,
                                     env=env, start_new_session=True)
        os.close(slave)
        self.captured = bytearray()

    def resize(self, rows, cols):
        fcntl.ioctl(self.master, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
        proc = getattr(self, "proc", None)
        if proc is not None and proc.poll() is None:
            # sshd delivers SIGWINCH itself; this harness has no controlling tty.
            os.kill(proc.pid, signal.SIGWINCH)

    def pump(self, seconds=0.1):
        deadline = time.monotonic() + seconds
        got = bytearray()
        while time.monotonic() < deadline:
            ready, _, _ = select.select([self.master], [], [], max(0, deadline - time.monotonic()))
            if not ready:
                break
            try:
                chunk = os.read(self.master, 65536)
            except OSError:
                break
            if not chunk:
                break
            got.extend(chunk)
        self.captured.extend(got)
        return bytes(got)

    def expect(self, text, timeout=15):
        deadline = time.monotonic() + timeout
        data = bytearray()
        while time.monotonic() < deadline:
            data.extend(self.pump(0.2))
            if text.encode() in data:
                return bytes(data)
            if self.proc.poll() is not None:
                break
        raise AssertionError(f"missing {text!r}; exit={self.proc.poll()}; tail={bytes(data[-400:])!r}")

    def send(self, data):
        os.write(self.master, data)

    def click(self, col, row):
        """1-based SGR press + release, as sent by xterm-compatible terminals over SSH."""
        self.send(f"\x1b[<0;{col};{row}M\x1b[<0;{col};{row}m".encode())

    def hover(self, col, row):
        self.send(f"\x1b[<35;{col};{row}M".encode())

    def finish(self, how="ctrl-c"):
        if how == "hangup":
            os.kill(self.proc.pid, signal.SIGHUP)
        else:
            self.send(b"\x03\x03")
        deadline = time.monotonic() + 10
        while self.proc.poll() is None and time.monotonic() < deadline:
            self.pump(0.1)
        status = self.proc.poll()
        self.pump(0.2)
        try:
            assert status == 0, f"exit status {status}"
            assert b"\x1b[?1049l" in self.captured, "alternate screen not restored"
            assert not list((self.root / "tmp").iterdir()), "temporary state not removed"
            assert not list(self.root.rglob("session.db")), "offline mode created device credentials"
        finally:
            if self.proc.poll() is None:
                self.proc.kill()
                self.proc.wait()
            os.close(self.master)
            self.tmp.cleanup()


def demo_flow(binary):
    s = Session(binary, ["-demo"])
    first = s.expect("Umbrella Devs")
    # SSH sessions usually lack COLORTERM: colours must still be 24-bit and filled.
    s.captured.extend(b"")
    assert b"48;2;" in first and b"38;2;" in first, "truecolor expected without COLORTERM"
    # Keep draining until the first frame is complete (it may span several reads).
    first += s.pump(0.5)
    assert b"DEMO" in first, "demo banner missing from the first frame"
    s.hover(40, 12)
    s.send(b"/")
    s.expect("Show keyboard and command help")
    s.send(b"help\r")
    s.expect("NAVIGATION")
    s.send(b"\x1b")
    s.expect("Umbrella Devs")
    s.send(b"a")
    s.expect("Send files")
    s.send(b"\x1b")
    s.expect("Umbrella Devs")
    s.click(5, 9)  # the "+" button in the navigation rail
    s.expect("Start a direct chat")
    s.send(b"\x1b")
    s.expect("Umbrella Devs")
    for rows, cols in ((30, 100), (20, 60), (42, 130), (50, 200)):
        s.resize(rows, cols)
        s.expect("Umbrella Devs")
        assert s.proc.poll() is None, "crashed while resizing"
    s.send(b"i")
    s.expect("Ctrl+J")
    s.send(b"PTY check \xf0\x9f\x91\x8c\r")
    s.expect("nothing transmitted")
    s.finish()
    print("PASS demo flow (truecolor, hover, slash commands, file browser, click, resize)")


def colors256(binary):
    s = Session(binary, ["-demo", "-colors", "256"])
    out = s.expect("Umbrella Devs")
    assert b"48;5;" in out and b"48;2;" not in out, "-colors 256 must use the 256-colour palette"
    s.finish()
    print("PASS -colors 256")


def hangup(binary):
    s = Session(binary, ["-demo"])
    s.expect("Umbrella Devs")
    s.finish("hangup")
    print("PASS dropped SSH connection (SIGHUP) exits cleanly")


def offline_flow(binary):
    s = Session(binary, ["-offline"])
    first = s.expect("Welcome to") + s.pump(0.3)
    assert b"Continue" in first and b"Exit" in first, "welcome buttons missing"
    s.send(b"\r")
    s.expect("Independent and unofficial")
    s.send(b" ")
    s.expect("I understand")
    s.send(b"\r")
    s.expect("Log in to WhatsApp")
    s.send(b"\x1b")
    s.expect("Disclaimer")
    assert b"No chats here yet" not in s.captured, "unpaired WhatsCLI workspace was rendered"
    s.finish()
    print("PASS offline onboarding (welcome, consent, link a device)")


if __name__ == "__main__":
    binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "./whats-cli").resolve())
    demo_flow(binary)
    colors256(binary)
    hangup(binary)
    offline_flow(binary)
