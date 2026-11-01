// Package ui draws WhatsCLI into a cell canvas. Every cell has an explicit
// foreground and background, so no segment can reset the page back to the
// terminal's default colour (the cause of "dull", unfilled panels).
package ui

import (
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Color is 0xRRGGBB with bit 24 set; the zero value means "unset/keep".
type Color uint32

func Hex(v uint32) Color  { return Color(v&0xFFFFFF) | 1<<24 }
func (c Color) Set() bool { return c>>24 != 0 }
func (c Color) rgb() (uint8, uint8, uint8) {
	return uint8(c >> 16), uint8(c >> 8), uint8(c)
}

// Mix blends a toward b by t (0..1).
func Mix(a, b Color, t float64) Color {
	if !a.Set() {
		return b
	}
	if !b.Set() {
		return a
	}
	ar, ag, ab := a.rgb()
	br, bg, bb := b.rgb()
	f := func(x, y uint8) uint32 { return uint32(float64(x)*(1-t) + float64(y)*t + .5) }
	return Hex(f(ar, br)<<16 | f(ag, bg)<<8 | f(ab, bb))
}

// Profile selects the colour escape format.
type Profile int

const (
	TrueColor Profile = iota
	Color256
)

var profile = TrueColor

// SetProfile must be called before drawing begins.
func SetProfile(p Profile) { profile = p }

// ParseProfile resolves -colors auto|truecolor|256. SSH sessions usually do
// not forward COLORTERM, so "auto" assumes truecolor unless the terminal is
// known to be limited.
func ParseProfile(mode string, getenv func(string) string) (Profile, bool) {
	switch strings.ToLower(mode) {
	case "truecolor", "24bit", "true":
		return TrueColor, true
	case "256":
		return Color256, true
	case "", "auto":
		ct := strings.ToLower(getenv("COLORTERM"))
		if ct == "truecolor" || ct == "24bit" {
			return TrueColor, true
		}
		switch getenv("TERM") {
		case "linux", "vt100", "vt220", "dumb", "ansi", "xterm", "screen":
			return Color256, true
		}
		return TrueColor, true
	}
	return TrueColor, false
}

type Cell struct {
	R      rune
	FG, BG Color
	Bold   bool
	Cont   bool // right half of a double-width rune
}

type Rect struct{ X, Y, W, H int }

// R builds a Rect from x, y, width and height.
func R(x, y, w, h int) Rect { return Rect{X: x, Y: y, W: w, H: h} }

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}
func (r Rect) Right() int  { return r.X + r.W }
func (r Rect) Bottom() int { return r.Y + r.H }
func (r Rect) Inset(n int) Rect {
	return Rect{r.X + n, r.Y + n, max(0, r.W-2*n), max(0, r.H-2*n)}
}
func (r Rect) Intersect(o Rect) Rect {
	x, y := max(r.X, o.X), max(r.Y, o.Y)
	x2, y2 := min(r.Right(), o.Right()), min(r.Bottom(), o.Bottom())
	if x2 <= x || y2 <= y {
		return Rect{}
	}
	return Rect{x, y, x2 - x, y2 - y}
}

var cond = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false // ambiguous glyphs (·, …, ◎) occupy one cell over SSH
	return c
}()

func RuneWidth(r rune) int { return cond.RuneWidth(r) }
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Canvas is nil-safe: with a nil canvas, drawing is skipped and only layout/hit
// registration runs, which is how mouse hit-testing reuses the renderer.
type Canvas struct {
	W, H  int
	cells []Cell
	clip  Rect
}

func NewCanvas(w, h int, bg Color) *Canvas {
	c := &Canvas{W: w, H: h, cells: make([]Cell, w*h), clip: Rect{0, 0, w, h}}
	for i := range c.cells {
		c.cells[i] = Cell{R: ' ', BG: bg}
	}
	return c
}

func (c *Canvas) Clip(r Rect) Rect {
	if c == nil {
		return Rect{}
	}
	old := c.clip
	c.clip = r.Intersect(Rect{X: 0, Y: 0, W: c.W, H: c.H})
	return old
}
func (c *Canvas) Restore(r Rect) {
	if c != nil {
		c.clip = r
	}
}

func (c *Canvas) Cell(x, y int) Cell {
	if c == nil || x < 0 || y < 0 || x >= c.W || y >= c.H {
		return Cell{}
	}
	return c.cells[y*c.W+x]
}

// repair prevents orphaned halves of double-width runes when overwriting.
func (c *Canvas) repair(x, y int) {
	i := y*c.W + x
	if c.cells[i].Cont && x > 0 {
		c.cells[i-1].R, c.cells[i-1].Cont = ' ', false
	}
	if x+1 < c.W && c.cells[i+1].Cont {
		c.cells[i+1].R, c.cells[i+1].Cont = ' ', false
	}
}

func (c *Canvas) Put(x, y int, r rune, fg, bg Color, bold bool) {
	if c == nil {
		return
	}
	w := RuneWidth(r)
	if w == 0 || !c.clip.Contains(x, y) || (w == 2 && !c.clip.Contains(x+1, y)) {
		return
	}
	c.repair(x, y)
	cell := &c.cells[y*c.W+x]
	cell.R, cell.Cont, cell.Bold = r, false, bold
	if fg.Set() {
		cell.FG = fg
	}
	if bg.Set() {
		cell.BG = bg
	}
	if w == 2 {
		c.repair(x+1, y)
		next := &c.cells[y*c.W+x+1]
		next.R, next.Cont, next.Bold = 0, true, bold
		next.BG = cell.BG
	}
}

// Text draws a single line and returns its width.
func (c *Canvas) Text(x, y int, s string, fg, bg Color, bold bool) int {
	w := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if rw == 0 {
			continue
		}
		c.Put(x+w, y, r, fg, bg, bold)
		w += rw
	}
	return w
}

func (c *Canvas) Fill(r Rect, bg Color) {
	if c == nil {
		return
	}
	r = r.Intersect(c.clip)
	for y := r.Y; y < r.Bottom(); y++ {
		for x := r.X; x < r.Right(); x++ {
			c.repair(x, y)
			cell := &c.cells[y*c.W+x]
			cell.R, cell.Cont, cell.Bold = ' ', false, false
			if bg.Set() {
				cell.BG = bg
			}
		}
	}
}

// Box draws a rounded border. A set fill also paints the interior and border
// cells; an unset fill leaves existing content untouched.
func (c *Canvas) Box(r Rect, border, fill Color) {
	if c == nil || r.W < 2 || r.H < 2 {
		return
	}
	if fill.Set() {
		c.Fill(r, fill)
	}
	for x := r.X + 1; x < r.Right()-1; x++ {
		c.Put(x, r.Y, '─', border, fill, false)
		c.Put(x, r.Bottom()-1, '─', border, fill, false)
	}
	for y := r.Y + 1; y < r.Bottom()-1; y++ {
		c.Put(r.X, y, '│', border, fill, false)
		c.Put(r.Right()-1, y, '│', border, fill, false)
	}
	c.Put(r.X, r.Y, '╭', border, fill, false)
	c.Put(r.Right()-1, r.Y, '╮', border, fill, false)
	c.Put(r.X, r.Bottom()-1, '╰', border, fill, false)
	c.Put(r.Right()-1, r.Bottom()-1, '╯', border, fill, false)
}

func (c *Canvas) HLine(x, y, w int, ch rune, fg Color) {
	for i := 0; i < w; i++ {
		c.Put(x+i, y, ch, fg, 0, false)
	}
}

// Plain returns text-only rows for tests and diagnostics.
func (c *Canvas) Plain() string {
	var b strings.Builder
	for y := 0; y < c.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < c.W; x++ {
			cell := c.cells[y*c.W+x]
			if !cell.Cont {
				b.WriteRune(cell.R)
			}
		}
	}
	return b.String()
}

func (c *Canvas) String() string {
	var b strings.Builder
	b.Grow(c.W*c.H*2 + c.H*24)
	for y := 0; y < c.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		var fg, bg Color
		bold := false
		for x := 0; x < c.W; x++ {
			cell := c.cells[y*c.W+x]
			if cell.Cont {
				continue
			}
			r := cell.R
			if r == 0 {
				r = ' '
			}
			needFG := r != ' ' && cell.FG != fg
			if needFG || cell.BG != bg || cell.Bold != bold {
				b.WriteString("\x1b[")
				first := true
				sep := func() {
					if !first {
						b.WriteByte(';')
					}
					first = false
				}
				if cell.Bold != bold {
					sep()
					if cell.Bold {
						b.WriteByte('1')
					} else {
						b.WriteString("22")
					}
					bold = cell.Bold
				}
				if needFG {
					sep()
					writeColor(&b, 38, cell.FG)
					fg = cell.FG
				}
				if cell.BG != bg {
					sep()
					writeColor(&b, 48, cell.BG)
					bg = cell.BG
				}
				b.WriteByte('m')
			}
			b.WriteRune(r)
		}
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func writeColor(b *strings.Builder, base int, c Color) {
	if !c.Set() {
		b.WriteString(strconv.Itoa(base + 1))
		return
	}
	r, g, bl := c.rgb()
	if profile == Color256 {
		b.WriteString(strconv.Itoa(base))
		b.WriteString(";5;")
		b.WriteString(strconv.Itoa(xterm256(r, g, bl)))
		return
	}
	b.WriteString(strconv.Itoa(base))
	b.WriteString(";2;")
	b.WriteString(strconv.Itoa(int(r)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(g)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(bl)))
}

func xterm256(r, g, b uint8) int {
	level := func(v uint8) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (int(v) - 35) / 40
	}
	ri, gi, bi := level(r), level(g), level(b)
	cube := 16 + 36*ri + 6*gi + bi
	steps := [...]int{0, 95, 135, 175, 215, 255}
	dist := func(r2, g2, b2 int) int {
		dr, dg, db := int(r)-r2, int(g)-g2, int(b)-b2
		return dr*dr + dg*dg + db*db
	}
	best, bestD := cube, dist(steps[ri], steps[gi], steps[bi])
	avg := (int(r) + int(g) + int(b)) / 3
	if gray := (avg - 8 + 5) / 10; gray >= 0 && gray < 24 {
		v := 8 + gray*10
		if d := dist(v, v, v); d < bestD {
			best = 232 + gray
		}
	}
	return best
}

// Truncate shortens s to w cells, appending tail when it was cut.
func Truncate(s string, w int, tail string) string {
	if w <= 0 {
		return ""
	}
	if StringWidth(s) <= w {
		return s
	}
	tw := StringWidth(tail)
	if tw >= w {
		tail, tw = "", 0
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if used+rw > w-tw {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + tail
}

// TailFit keeps the end of s (the caret side of an input) within w cells. It
// scans backwards once, so very long drafts stay cheap to redraw.
func TailFit(s string, w int) string {
	runes := []rune(s)
	used := 0
	i := len(runes)
	for i > 0 {
		rw := RuneWidth(runes[i-1])
		if used+rw > w {
			break
		}
		used += rw
		i--
	}
	return string(runes[i:])
}

func splitWidth(s string, w int) (string, string) {
	used := 0
	for i, r := range s {
		rw := RuneWidth(r)
		if used+rw > w && i > 0 {
			return s[:i], s[i:]
		}
		used += rw
	}
	return s, ""
}

// Wrap word-wraps text to w cells, hard-breaking overlong words.
func Wrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line, lw := "", 0
		for _, word := range words {
			ww := StringWidth(word)
			for ww > w {
				if lw > 0 {
					out = append(out, line)
					line, lw = "", 0
				}
				head, rest := splitWidth(word, w)
				out = append(out, head)
				word, ww = rest, StringWidth(rest)
			}
			if ww == 0 {
				continue
			}
			switch {
			case lw == 0:
				line, lw = word, ww
			case lw+1+ww <= w:
				line += " " + word
				lw += 1 + ww
			default:
				out = append(out, line)
				line, lw = word, ww
			}
		}
		out = append(out, line)
	}
	return out
}

// Dim darkens a region toward color to (used behind modal dialogs).
func (c *Canvas) Dim(r Rect, to Color, t float64) {
	if c == nil {
		return
	}
	r = r.Intersect(Rect{X: 0, Y: 0, W: c.W, H: c.H})
	for y := r.Y; y < r.Bottom(); y++ {
		for x := r.X; x < r.Right(); x++ {
			cell := &c.cells[y*c.W+x]
			cell.FG, cell.BG = Mix(cell.FG, to, t), Mix(cell.BG, to, t)
		}
	}
}
