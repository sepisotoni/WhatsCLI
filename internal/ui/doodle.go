package ui

// Doodle paints the faint chat wallpaper (outlined glyph "doodles") behind
// panels. It is deterministic so redraws never shimmer.
func Doodle(c *Canvas, r Rect, ink Color) {
	if c == nil {
		return
	}
	glyphs := []rune("∘◌✦◇☆♪♡✎⌂☾✧⌘")
	r = r.Intersect(c.clip)
	for y := r.Y; y < r.Bottom(); y++ {
		for x := r.X; x < r.Right(); x++ {
			h := uint32(x)*2654435761 ^ uint32(y)*2246822519
			h ^= h >> 15
			h *= 2246822519
			h ^= h >> 13
			if x%2 == 0 && h%13 == 0 {
				c.Put(x, y, glyphs[int(h>>8)%len(glyphs)], ink, 0, false)
			}
		}
	}
}
