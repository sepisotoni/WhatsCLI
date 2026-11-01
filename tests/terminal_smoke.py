"""Linux-only offline PTY acceptance check. Build the binary before running."""
import fcntl
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time


def scenario(binary, arguments, actions):
    with tempfile.TemporaryDirectory(prefix="whatscli-pty-") as directory:
        root = Path(directory)
        (root / "tmp").mkdir()
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 42, 130, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                   XDG_CONFIG_HOME=str(root / "config"),
                   TMPDIR=str(root / "tmp"))
        process = subprocess.Popen([binary, *arguments], stdin=slave,
                                   stdout=slave, stderr=slave, env=env,
                                   start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait_for(text):
            deadline = time.monotonic() + 15
            data = bytearray()
            while time.monotonic() < deadline:
                ready, _, _ = select.select(
                    [master], [], [], max(0, deadline - time.monotonic()))
                if ready:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError:
                        break
                    if not chunk:
                        break
                    captured.extend(chunk)
                    data.extend(chunk)
                    if text.encode() in data:
                        return
                if process.poll() is not None:
                    break
            raise AssertionError(
                f"Missing terminal text {text!r}; exit={process.poll()}; "
                f"synthetic-output tail={bytes(data[-800:])!r}")

        try:
            for keys, expected in actions:
                if keys:
                    os.write(master, keys)
                if expected:
                    wait_for(expected)
            os.write(master, b"\x03")
            # Continue consuming redraws while quitting. Waiting without reading
            # can fill the PTY buffer and block a legitimate terminal writer.
            deadline = time.monotonic() + 10
            while process.poll() is None and time.monotonic() < deadline:
                ready, _, _ = select.select([master], [], [], 0.1)
                if ready:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError:
                        break
                    captured.extend(chunk)
            result = process.wait(timeout=max(0.1, deadline - time.monotonic()))
            # Drain shutdown output, including alternate-screen restoration.
            while select.select([master], [], [], 0)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                captured.extend(chunk)
            assert result == 0, f"Exit status {result}"
            assert b"\x1b[?1049l" in captured, "Alternate screen not restored"
            assert not list((root / "tmp").iterdir()), "Demo state not removed"
            assert not list(root.rglob("session.db")), "Offline device credentials created"
            print("PASS PTY:", " ".join(arguments))
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)


if __name__ == "__main__":
    binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "./whats-cli").resolve())
    scenario(binary, ["-demo"], [
        (b"", "Umbrella Devs"),
        (b"i", "Ctrl+J"),
        (b"PTY offline check\r", "nothing transmitted"),
        # SGR mouse press on the navigation rail's + control (1-based cells).
        (b"\x1b[<0;3;7M", "NEW DIRECT CHAT"),
        (b"\x1b", "Umbrella Devs"),
    ])
    scenario(binary, ["-offline"], [
        (b"", "Welcome to"),
        (b"\r", "Independent and unofficial"),
        (b" ", "[x]"),
        (b"\r", "LINK A DEVICE"),
        (b"i", "No matching chats"),
    ])
