package ui

import (
	"errors"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCode draws pairing material as white/black half blocks. It never renders
// the payload as text.
type QRCode struct{ bits [][]bool }

const quiet = 2 // modules of white margin; scanners tolerate this over the nominal 4

// NewQR uses the lowest error-correction level so WhatsApp's long payload
// stays small enough to fit beside the instructions without full screen.
func NewQR(payload string) (*QRCode, error) {
	q, err := qrcode.New(payload, qrcode.Low)
	if err != nil {
		return nil, err
	}
	all := q.Bitmap()
	const lib = 4 // the encoder adds its own 4-module border
	if len(all) <= 2*lib {
		return nil, errors.New("invalid QR bitmap")
	}
	bits := make([][]bool, 0, len(all)-2*lib)
	for _, row := range all[lib : len(all)-lib] {
		bits = append(bits, row[lib:len(row)-lib])
	}
	return &QRCode{bits: bits}, nil
}

// Size is the cell footprint including the quiet zone.
func (q *QRCode) Size() (cols, rows int) {
	n := len(q.bits) + 2*quiet
	return n, (n + 1) / 2
}

func (q *QRCode) dark(x, y int) bool {
	x, y = x-quiet, y-quiet
	return y >= 0 && x >= 0 && y < len(q.bits) && x < len(q.bits[y]) && q.bits[y][x]
}

func (q *QRCode) Draw(c *Canvas, x, y int) {
	if c == nil {
		return
	}
	black, white := Hex(0), Hex(0xFFFFFF)
	pick := func(d bool) Color {
		if d {
			return black
		}
		return white
	}
	cols, rows := q.Size()
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			c.Put(x+col, y+row, '▀', pick(q.dark(col, 2*row)), pick(q.dark(col, 2*row+1)), false)
		}
	}
}
