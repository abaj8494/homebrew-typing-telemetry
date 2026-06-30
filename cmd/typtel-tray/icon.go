//go:build linux

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
)

// trayIcon renders a small "keyboard" glyph to PNG bytes for the StatusNotifier
// item: a dark rounded panel with three rows of light keycaps and a wide
// spacebar. Drawn in code so the binary carries no asset files.
func trayIcon() []byte {
	const size = 32
	bg := color.NRGBA{0x1e, 0x1e, 0x2e, 0xff}  // panel
	key := color.NRGBA{0xcd, 0xd6, 0xf4, 0xff} // keycap
	none := color.NRGBA{0, 0, 0, 0}            // transparent

	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// Transparent background with a rounded dark panel inset by 2px.
	const r = 5
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := none
			if inRoundRect(x, y, 2, 2, size-2, size-2, r) {
				c = bg
			}
			img.SetNRGBA(x, y, c)
		}
	}

	// Three rows of 4 keycaps.
	drawKey := func(x, y, w, h int) {
		for j := y; j < y+h; j++ {
			for i := x; i < x+w; i++ {
				img.SetNRGBA(i, j, key)
			}
		}
	}
	startX, startY := 6, 7
	const kw, kh, gap = 4, 4, 2
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			drawKey(startX+col*(kw+gap), startY+row*(kh+gap), kw, kh)
		}
	}
	// Spacebar.
	drawKey(8, startY+3*(kh+gap), size-16, 3)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("encode icon: %v", err)
		return nil
	}
	return buf.Bytes()
}

// inRoundRect reports whether (x,y) is inside the rectangle [x0,x1)×[y0,y1)
// with corners rounded at radius r.
func inRoundRect(x, y, x0, y0, x1, y1, r int) bool {
	if x < x0 || x >= x1 || y < y0 || y >= y1 {
		return false
	}
	// Corner centres.
	cx, cy := x, y
	switch {
	case x < x0+r && y < y0+r:
		cx, cy = x0+r, y0+r
	case x >= x1-r && y < y0+r:
		cx, cy = x1-r-1, y0+r
	case x < x0+r && y >= y1-r:
		cx, cy = x0+r, y1-r-1
	case x >= x1-r && y >= y1-r:
		cx, cy = x1-r-1, y1-r-1
	default:
		return true // not in a corner region
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}
