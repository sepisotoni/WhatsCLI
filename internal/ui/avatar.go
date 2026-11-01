package ui

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // WhatsApp serves JPEG profile pictures
	_ "image/png"
	"strings"
	"unicode"
)

const (
	AvatarW = 8 // cells
	AvatarH = 3 // cells (8x6 half-block pixels, approximately square)
)

// Photo is a profile picture reduced to 8x6 half-block pixels.
type Photo struct{ px [6][8]Color }

func PhotoFromColors(px [6][8]Color) *Photo { return &Photo{px} }

// DecodePhoto center-crops and box-averages an image. Size is checked first so a
// hostile image cannot cause a decompression bomb.
func DecodePhoto(data []byte) (*Photo, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > 16<<20 {
		return nil, errors.New("unsupported image size")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	p := &Photo{}
	for py := 0; py < 6; py++ {
		for pxl := 0; pxl < 8; pxl++ {
			sx0, sx1 := x0+pxl*side/8, x0+(pxl+1)*side/8
			sy0, sy1 := y0+py*side/6, y0+(py+1)*side/6
			var r, g, bl, n uint32
			step := max(1, (sx1-sx0)/8)
			for y := sy0; y < max(sy1, sy0+1); y += step {
				for x := sx0; x < max(sx1, sx0+1); x += step {
					cr, cg, cb, _ := img.At(x, y).RGBA()
					r, g, bl, n = r+cr>>8, g+cg>>8, bl+cb>>8, n+1
				}
			}
			p.px[py][pxl] = Hex(r/n<<16 | g/n<<8 | bl/n)
		}
	}
	return p, nil
}

var avatarFills = []uint32{0x0D5A3E, 0x0B5555, 0x146B4A, 0x0F5A5E, 0x0C4F3A, 0x137063}

// Initials returns up to two letters; numbers and symbols fall back to "#".
func Initials(name string) string {
	var out []rune
	words := strings.Fields(name)
	for _, word := range words {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "#"
	}
	if len(words) == 1 && unicode.IsDigit(out[0]) {
		return "#"
	}
	return string(out)
}

func inCircle(x, y int) bool {
	dx, dy := float64(x)+.5-4, float64(y)+.5-3
	return dx*dx/16+dy*dy/9 <= 1.05
}

// DrawAvatar draws a round avatar: the photo when present, otherwise a tinted
// circle containing the contact's initials. bg is what shows in the corners.
func DrawAvatar(c *Canvas, x, y int, name string, photo *Photo, bg Color) {
	if c == nil {
		return
	}
	h := uint32(0)
	for _, r := range name {
		h = h*31 + uint32(r)
	}
	fill := Hex(avatarFills[h%uint32(len(avatarFills))])
	letters := []rune(Initials(name))
	pixel := func(px, py int) Color {
		if photo != nil {
			return photo.px[py][px]
		}
		return fill
	}
	for row := 0; row < AvatarH; row++ {
		for col := 0; col < AvatarW; col++ {
			top, bottom := inCircle(col, 2*row), inCircle(col, 2*row+1)
			switch {
			case top && bottom:
				if photo == nil && row == 1 && col >= 3 && col-3 < len(letters) {
					c.Put(x+col, y+row, letters[col-3], Accent, fill, true)
				} else {
					c.Put(x+col, y+row, '▀', pixel(col, 2*row), pixel(col, 2*row+1), false)
				}
			case top:
				c.Put(x+col, y+row, '▀', pixel(col, 2*row), bg, false)
			case bottom:
				c.Put(x+col, y+row, '▄', pixel(col, 2*row+1), bg, false)
			default:
				c.Put(x+col, y+row, ' ', 0, bg, false)
			}
		}
	}
}
