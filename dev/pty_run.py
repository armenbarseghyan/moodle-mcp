#!/usr/bin/env python3
"""pty_run.py PROMPT=ANSWER... -- CMD [ARGS...]

Runs CMD in a pseudo-terminal (so `[ -t 0 ]` and `read -s` behave as for a
person typing), answers each PROMPT with ANSWER when it appears, copies the
output to stdout and exits with CMD's status. Used by dev/setup_test.sh;
`script` cannot be used because macOS's version hangs up the session as soon
as its piped stdin ends.
"""
import os
import pty
import sys
import termios
import time


def settle(fd: int) -> None:
    """Waits (up to 2 s) for the terminal to switch echo off, as bash's
    `read -s` does just after printing its prompt; a person never types that
    fast, and answering earlier would show the password on screen."""
    deadline = time.monotonic() + 2
    while time.monotonic() < deadline and termios.tcgetattr(fd)[3] & termios.ECHO:
        time.sleep(0.02)


def main() -> int:
    sep = sys.argv.index("--")
    answers = [a.split("=", 1) for a in sys.argv[1:sep]]
    cmd = sys.argv[sep + 1 :]
    pid, fd = pty.fork()
    if pid == 0:
        os.execvp(cmd[0], cmd)
    seen = b""
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        try:
            chunk = os.read(fd, 4096)
        except OSError:  # EIO: the child closed the terminal
            break
        if not chunk:
            break
        sys.stdout.buffer.write(chunk)
        sys.stdout.buffer.flush()
        seen += chunk
        # PowerShell asks the terminal for the cursor position before reading
        # input and waits for the answer; reply like a real terminal would.
        for _ in range(chunk.count(b"\x1b[6n")):
            os.write(fd, b"\x1b[1;1R")
        if answers and answers[0][0].encode() in seen:
            seen = b""
            settle(fd)
            os.write(fd, answers.pop(0)[1].encode() + b"\n")
    _, status = os.waitpid(pid, 0)
    return os.waitstatus_to_exitcode(status)


if __name__ == "__main__":
    sys.exit(main())
