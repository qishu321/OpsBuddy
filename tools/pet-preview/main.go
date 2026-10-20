// Renders the original companion art for visual inspection, without a native window.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"log"
	"opsbuddy/internal/pet"
	"os"
	"strings"
)

func main() {
	out := ".cache/pet-preview.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if len(os.Args) > 2 && os.Args[2] == "hover" {
		frame := image.NewRGBA(image.Rect(0, 0, pet.Width, pet.Height))
		pet.DrawCompanion(frame, "hover", 0, pet.Overlay{Controls: true})
		canvas := image.NewRGBA(frame.Bounds())
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{237, 242, 248, 255}}, image.Point{}, draw.Src)
		draw.Draw(canvas, canvas.Bounds(), frame, image.Point{}, draw.Over)
		f, err := os.Create(out)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, canvas); err != nil {
			log.Fatal(err)
		}
		return
	}
	if strings.HasSuffix(out, ".gif") {
		preview := &gif.GIF{LoopCount: 0}
		palette := color.Palette{color.RGBA{237, 242, 248, 255}}
		frames := []*image.RGBA{}
		seen := map[color.Color]bool{palette[0]: true}
		for _, phase := range []string{"idle", "coding", "petting", "hover-left", "hover-right"} {
			count := 12
			if phase == "idle" {
				count = 48
			}
			for n := 0; n < count; n++ {
				frame := pet.Frame(phase, n, false)
				cropped := image.NewRGBA(image.Rect(0, 0, 200, 160))
				draw.Draw(cropped, cropped.Bounds(), frame, image.Point{48, 78}, draw.Src)
				frame = cropped
				frames = append(frames, frame)
				for y := 0; y < frame.Bounds().Dy(); y++ {
					for x := 0; x < frame.Bounds().Dx(); x++ {
						c := frame.RGBAAt(x, y)
						if c.A == 255 && !seen[c] {
							palette = append(palette, c)
							seen[c] = true
						}
					}
				}
			}
		}
		for _, frame := range frames {
			p := image.NewPaletted(frame.Bounds(), palette)
			draw.Draw(p, p.Bounds(), &image.Uniform{C: palette[0]}, image.Point{}, draw.Src)
			draw.Draw(p, p.Bounds(), frame, image.Point{}, draw.Over)
			preview.Image = append(preview.Image, p)
			preview.Delay = append(preview.Delay, 17)
		}
		f, err := os.Create(out)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err = gif.EncodeAll(f, preview); err != nil {
			log.Fatal(err)
		}
		return
	}
	canvas := image.NewRGBA(image.Rect(0, 0, pet.Width*5, pet.Height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{237, 242, 248, 255}}, image.Point{}, draw.Src)
	for i, phase := range []string{"idle", "working", "waiting", "done", "error"} {
		draw.Draw(canvas, image.Rect(i*pet.Width, 0, (i+1)*pet.Width, pet.Height), pet.Frame(phase, 1, true), image.Point{}, draw.Over)
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, canvas); err != nil {
		log.Fatal(err)
	}
}
