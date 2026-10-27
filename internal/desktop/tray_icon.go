package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func trayIcon() []byte {
	return trayIconColor(color.RGBA{33, 185, 147, 255})
}

func trayIconColor(fill color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 3; y < 29; y++ {
		for x := 3; x < 29; x++ {
			img.Set(x, y, fill)
		}
	}
	for y := 9; y < 23; y++ {
		for x := 8; x < 24; x++ {
			if y < 12 || y > 19 || x < 11 || x > 20 {
				img.Set(x, y, color.RGBA{11, 30, 37, 255})
			}
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}
