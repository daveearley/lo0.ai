"""Drive `lo0 ui` in a pseudo-terminal and save each screen as styled runs.

usage: capture.py LO0 HOME OUTDIR COLS ROWS name:keys [name:keys ...]

Needs pyte (pip install pyte).

keys are separated by commas; named keys: enter, tab, esc, down, up, ctrlk,
wait (an extra second).
"""
import fcntl
import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import time

import pyte

lo0, home, out, cols, rows = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4]), int(sys.argv[5])
shots = [a.split(":", 1) for a in sys.argv[6:]]

named = {"enter": "\r", "tab": "\t", "esc": "\x1b", "down": "\x1b[B", "up": "\x1b[A", "ctrlk": "\x0b",
         "right": "\x1b[C", "left": "\x1b[D", "space": " "}

class Screen(pyte.Screen):
    """pyte without the tab movements Bubble Tea uses to skip blank cells."""

    def cursor_back_tab(self, count=None, *args, **kwargs):
        for _ in range(count or 1):
            stops = [t for t in self.tabstops if t < self.cursor.x]
            self.cursor.x = max(stops) if stops else 0

    def cursor_forward_tab(self, count=None, *args, **kwargs):
        for _ in range(count or 1):
            self.tab()


class Stream(pyte.ByteStream):
    csi = dict(pyte.ByteStream.csi, Z="cursor_back_tab", I="cursor_forward_tab")


screen = Screen(cols, rows)
stream = Stream(screen)

pid, fd = pty.fork()
if pid == 0:
    env = dict(os.environ, HOME=home, TERM="xterm-256color", COLORTERM="truecolor")
    env.pop("NO_COLOR", None)
    os.execve(lo0, [lo0, "ui"], env)

fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def pump(seconds):
    deadline = time.time() + seconds
    while time.time() < deadline:
        r, _, _ = select.select([fd], [], [], 0.05)
        if fd in r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                return
            # Answer background-colour queries as a dark terminal would.
            if b"\x1b]11;?" in data:
                os.write(fd, b"\x1b]11;rgb:0f0f/1111/1414\x1b\\")
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")
            stream.feed(data)


palette = []


def colour(c):
    if c == "default":
        return 0
    if c not in palette:
        palette.append(c)
    return palette.index(c) + 1


def snapshot():
    """Rows of [text, fg, bg, flags] runs; colours index the palette, 0 is the
    terminal default. Flags: 1 bold, 2 italic, 4 underline, 8 reverse."""
    lines = []
    for y in range(rows):
        row = screen.buffer[y]
        runs = []
        for x in range(cols):
            c = row[x]
            style = (c.fg, c.bg, c.bold, c.italics, c.underscore, c.reverse)
            ch = c.data or " "
            if runs and runs[-1][0] == style:
                runs[-1][1] += ch
            else:
                runs.append([style, ch])
        # Trailing blanks carry nothing a reader can see.
        while runs and not runs[-1][1].strip() and runs[-1][0][1] == "default" and not runs[-1][0][5]:
            runs.pop()
        lines.append([[t, colour(s[0]), colour(s[1]), s[2] | s[3] << 1 | s[4] << 2 | s[5] << 3] for s, t in runs])
    while lines and not lines[-1]:
        lines.pop()
    return lines


pump(6)
os.makedirs(out, exist_ok=True)
for name, keys in shots:
    for k in [k for k in keys.split(",") if k]:
        if k == "wait":
            pump(1.0)
            continue
        os.write(fd, named.get(k, k).encode())
        pump(0.6)
    pump(1.2)
    # Bubble Tea redraws only changed cells, which a capture can catch half
    # done; a resize makes it repaint the whole screen.
    for w in (cols - 1, cols):
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, w, 0, 0))
        os.kill(pid, signal.SIGWINCH)
        screen.resize(rows, w)
        pump(0.8)
    with open(os.path.join(out, name + ".json"), "w") as f:
        lines = snapshot()
        json.dump({"cols": cols, "rows": rows, "palette": ["#" + c for c in palette], "lines": lines}, f, separators=(",", ":"))
    print(name)

os.write(fd, b"q")
pump(0.5)
