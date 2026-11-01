#!/usr/bin/env python3
"""Render WhatsCLI's ANSI output (SGR 1/22/38/39/48/49/0 only) to a PNG so the
UI can be compared against the reference mock-ups without a real terminal.
Usage: ansi2png.py input.ansi output.png [cell_w cell_h]"""
import re
import sys
import unicodedata
from PIL import Image, ImageDraw, ImageFont

CW = int(sys.argv[3]) if len(sys.argv) > 3 else 10
CH = int(sys.argv[4]) if len(sys.argv) > 4 else 20
FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
size = int(CH * 0.72)
fonts = {False: ImageFont.truetype(FONT, size), True: ImageFont.truetype(BOLD, size)}
fallback = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", size)
emoji_cache = {}

QUAD = {  # block element -> filled quadrants (tl, tr, bl, br)
    "▀": (1, 1, 0, 0), "▄": (0, 0, 1, 1), "█": (1, 1, 1, 1),
    "▗": (0, 0, 0, 1), "▖": (0, 0, 1, 0), "▝": (0, 1, 0, 0), "▘": (1, 0, 0, 0),
    "▌": "left", "▍": "left3",
}

def width(ch):
    if unicodedata.combining(ch) or ch in "\u200d\ufe0f":
        return 0
    return 2 if unicodedata.east_asian_width(ch) in "WF" else 1

def parse(text):
    rows, fg, bg, bold = [], None, None, False
    for line in text.split("\n"):
        row, i = [], 0
        while i < len(line):
            if line[i] == "\x1b":
                m = re.match(r"\x1b\[([0-9;]*)m", line[i:])
                if not m:
                    i += 1
                    continue
                p = [int(x) if x else 0 for x in m.group(1).split(";")]
                j = 0
                while j < len(p):
                    c = p[j]
                    if c == 0: fg = bg = None; bold = False
                    elif c == 1: bold = True
                    elif c == 22: bold = False
                    elif c == 39: fg = None
                    elif c == 49: bg = None
                    elif c in (38, 48) and p[j+1:j+2] == [2]:
                        col = tuple(p[j+2:j+5])
                        if c == 38: fg = col
                        else: bg = col
                        j += 4
                    elif c in (38, 48) and p[j+1:j+2] == [5]:
                        n = p[j+2]
                        col = (n, n, n)
                        if c == 38: fg = col
                        else: bg = col
                        j += 2
                    j += 1
                i += m.end()
                continue
            ch = line[i]
            w = width(ch)
            if w:
                row.append((ch, fg, bg, bold))
                if w == 2:
                    row.append(("", fg, bg, bold))
            i += 1
        rows.append(row)
    return rows

def main():
    rows = parse(open(sys.argv[1], encoding="utf-8").read())
    W = max(len(r) for r in rows)
    img = Image.new("RGB", (W * CW, len(rows) * CH), (0, 0, 0))
    d = ImageDraw.Draw(img)
    for y, row in enumerate(rows):
        for x, (ch, fg, bg, bold) in enumerate(row):
            x0, y0 = x * CW, y * CH
            span = CW * (2 if x + 1 < len(row) and row[x + 1][0] == "" else 1)
            if bg:
                d.rectangle([x0, y0, x0 + span - 1, y0 + CH - 1], fill=bg)
            if not ch or ch == " ":
                continue
            col = fg or (200, 200, 200)
            if ch in QUAD:
                q = QUAD[ch]
                hw, hh = CW // 2, CH // 2
                if q == "left":
                    d.rectangle([x0, y0, x0 + hw - 1, y0 + CH - 1], fill=col)
                elif q == "left3":
                    d.rectangle([x0, y0, x0 + CW * 3 // 8, y0 + CH - 1], fill=col)
                else:
                    for k, (qx, qy) in enumerate([(0, 0), (1, 0), (0, 1), (1, 1)]):
                        if q[k]:
                            d.rectangle([x0 + qx * hw, y0 + qy * hh, x0 + qx * hw + (CW - hw if qx else hw) - 1, y0 + qy * hh + (CH - hh if qy else hh) - 1], fill=col)
                continue
            if ch in "─│╭╮╰╯┏┓┗┛":
                cx, cy = x0 + CW // 2, y0 + CH // 2
                t = 2 if ch in "┏┓┗┛" else 1
                h = ch in "─╭╮╰╯┏┓┗┛"
                v = ch in "│╭╮╰╯┏┓┗┛"
                if ch in "─":
                    d.rectangle([x0, cy, x0 + CW - 1, cy + t - 1], fill=col)
                elif ch == "│":
                    d.rectangle([cx, y0, cx + t - 1, y0 + CH - 1], fill=col)
                else:
                    left = ch in "╮╯┓┛"
                    up = ch in "╰╯┗┛"
                    hx = [x0, cx + t] if left else [cx, x0 + CW - 1]
                    vy = [y0, cy + t] if up else [cy, y0 + CH - 1]
                    d.rectangle([min(hx), cy, max(hx), cy + t - 1], fill=col)
                    d.rectangle([cx, min(vy), cx + t - 1, max(vy)], fill=col)
                continue
            f = fonts[bold]
            if f.getmask(ch).getbbox() is None:
                f = fallback
            d.text((x0, y0 + 1), ch, font=f, fill=col)
    img.save(sys.argv[2])

main()
