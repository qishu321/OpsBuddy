package pet

import (
	"image"
	"image/color"
	"math"
)

const Width, Height = 288, 278

const ControlsTop, ControlsLeft, ControlsRight, ControlsHeight = 240, 82, 206, 30

type Overlay struct{ Bubble, Controls, Paused bool }

// Smooth speech/UI surfaces contrast with the deliberately crisp pixel cat.
// Coverage is calculated only for these small shapes, with premultiplied alpha.
func roundedShape(m *image.RGBA, x, y, w, h, r float64, c color.NRGBA) {
	for py := max(0, int(math.Floor(y))-1); py < min(m.Bounds().Dy(), int(math.Ceil(y+h))+1); py++ {
		for px := max(0, int(math.Floor(x))-1); px < min(m.Bounds().Dx(), int(math.Ceil(x+w))+1); px++ {
			qx := math.Abs(float64(px)+.5-x-w/2) - (w/2 - r)
			qy := math.Abs(float64(py)+.5-y-h/2) - (h/2 - r)
			distance := math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - r
			a := max(0, min(1, .5-distance)) * float64(c.A) / 255
			if a <= 0 {
				continue
			}
			d := m.RGBAAt(px, py)
			m.SetRGBA(px, py, color.RGBA{uint8(math.Round(float64(c.R)*a + float64(d.R)*(1-a))), uint8(math.Round(float64(c.G)*a + float64(d.G)*(1-a))), uint8(math.Round(float64(c.B)*a + float64(d.B)*(1-a))), uint8(math.Round(255*a + float64(d.A)*(1-a)))})
		}
	}
}

// Original pixel geometry, rendered into a reused buffer in the native window.
func Frame(phase string, frame int, bubble bool) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, Width, Height))
	DrawFrame(m, phase, frame, bubble)
	return m
}
func DrawFrame(m *image.RGBA, phase string, frame int, bubble bool) {
	DrawCompanion(m, phase, frame, Overlay{Bubble: bubble})
}
func DrawCompanion(m *image.RGBA, phase string, frame int, overlay Overlay) {
	clear(m.Pix)
	rect := func(x, y, w, h int, c color.RGBA) {
		for py := max(0, y); py < min(Height, y+h); py++ {
			for px := max(0, x); px < min(Width, x+w); px++ {
				m.SetRGBA(px, py, c)
			}
		}
	}
	if overlay.Bubble {
		roundedShape(m, 13, 13, 262, 66, 15, color.NRGBA{38, 54, 76, 9})
		roundedShape(m, 14, 12, 260, 66, 14, color.NRGBA{38, 54, 76, 12})
		roundedShape(m, 14, 10, 260, 66, 14, color.NRGBA{224, 231, 238, 255})
		roundedShape(m, 15, 11, 258, 64, 13, color.NRGBA{255, 255, 255, 255})
		for y := 0; y < 6; y++ {
			rect(138+y, 75+y, 12-y*2, 1, color.RGBA{255, 255, 255, 255})
		}
	}
	if overlay.Controls {
		roundedShape(m, ControlsLeft-2, ControlsTop+2, 128, 30, 15, color.NRGBA{38, 54, 76, 14})
		roundedShape(m, ControlsLeft, ControlsTop, 124, 30, 15, color.NRGBA{228, 233, 240, 255})
		roundedShape(m, ControlsLeft+1, ControlsTop+1, 122, 28, 14, color.NRGBA{255, 255, 255, 255})
		rect(143, ControlsTop+7, 1, 16, color.RGBA{231, 235, 240, 255})
		// Chat and pause/play icons, kept secondary to the cat.
		roundedShape(m, 105, ControlsTop+8, 16, 12, 3, color.NRGBA{79, 92, 107, 255})
		roundedShape(m, 106.5, ControlsTop+9.5, 13, 9, 1.5, color.NRGBA{255, 255, 255, 255})
		rect(107, ControlsTop+19, 2, 3, color.RGBA{79, 92, 107, 255})
		if overlay.Paused {
			for x := 0; x < 9; x++ {
				height := min(x+1, 9-x)
				rect(171+x, ControlsTop+15-height, 1, height*2, color.RGBA{79, 92, 107, 255})
			}
		} else {
			roundedShape(m, 170, ControlsTop+9, 2.5, 12, 1, color.NRGBA{79, 92, 107, 255})
			roundedShape(m, 177, ControlsTop+9, 2.5, 12, 1, color.NRGBA{79, 92, 107, 255})
		}
	}
	ink := color.RGBA{56, 43, 44, 255}
	fur := color.RGBA{195, 134, 76, 255}
	shade := color.RGBA{153, 94, 57, 255}
	gold := color.RGBA{226, 176, 112, 255}
	cream := color.RGBA{255, 235, 202, 255}
	white := color.RGBA{255, 248, 232, 255}
	pink := color.RGBA{231, 143, 139, 255}
	mint := color.RGBA{87, 170, 156, 255}
	t := frame % 12
	idle := phase == "idle"
	idleCycle := frame % 48
	stretch := idle && idleCycle >= 16 && idleCycle < 22
	wave := idle && idleCycle >= 32 && idleCycle < 38
	bounce := 0
	if phase == "done" || phase == "petting" {
		bounce = -int(math.Abs(math.Sin(float64(t)*math.Pi/6)) * 4)
	}
	if phase == "hover" || phase == "hover-left" || phase == "hover-right" {
		bounce = -1
	}
	if idle {
		bounce = -int(math.Round((1 + math.Sin(float64(t)*math.Pi/6)) * 1.5))
	}
	if stretch {
		bounce -= 3
	}
	ox, oy := 76, 92+bounce
	if idle && idleCycle >= 24 && idleCycle < 30 {
		ox += 2
	}
	px := func(x, y, w, h int, c color.RGBA) { rect(ox+x*2, oy+y*2, w*2, h*2, c) }
	ellipse := func(x, y, w, h int, c color.RGBA) {
		for yy := 0; yy < h; yy++ {
			for xx := 0; xx < w; xx++ {
				dx, dy := (float64(xx)+.5-float64(w)/2)/(float64(w)/2), (float64(yy)+.5-float64(h)/2)/(float64(h)/2)
				if dx*dx+dy*dy <= 1 {
					px(x+xx, y+yy, 1, 1, c)
				}
			}
		}
	}
	poly := func(points [][2]int, c color.RGBA) {
		minY, maxY := points[0][1], points[0][1]
		for _, p := range points {
			minY = min(minY, p[1])
			maxY = max(maxY, p[1])
		}
		for y := minY; y <= maxY; y++ {
			for x := 0; x < 70; x++ {
				inside := false
				j := len(points) - 1
				for i, p := range points {
					q := points[j]
					if (p[1] > y) != (q[1] > y) && float64(x) < float64(q[0]-p[0])*float64(y-p[1])/float64(q[1]-p[1])+float64(p[0]) {
						inside = !inside
					}
					j = i
				}
				if inside {
					px(x, y, 1, 1, c)
				}
			}
		}
	}
	// Ground, a curled articulated tail, and a sitting body.
	ellipse(4, 64, 59, 5, color.RGBA{58, 63, 67, 95})
	tail := int(math.Sin(float64(t)*math.Pi/6) * 2)
	ellipse(0, 48-tail, 30, 16, ink)
	ellipse(3, 49-tail, 25, 11, shade)
	ellipse(6, 50-tail, 18, 8, gold)
	ellipse(7, 51-tail, 10, 6, white)
	ellipse(14, 29, 42, 37, ink)
	ellipse(17, 30, 36, 34, fur)
	ellipse(22, 34, 25, 29, cream)
	ellipse(26, 35, 17, 25, white)
	px(17, 40, 5, 4, shade)
	px(17, 49, 5, 4, shade)
	px(48, 40, 5, 4, shade)
	px(48, 49, 5, 4, shade)
	ellipse(13, 58, 21, 9, ink)
	ellipse(15, 58, 18, 7, fur)
	ellipse(16, 61, 15, 4, white)
	ellipse(39, 58, 19, 9, ink)
	ellipse(40, 58, 16, 7, fur)
	ellipse(41, 61, 14, 4, white)
	// Large ears and the tabby head silhouette.
	poly([][2]int{{11, 25}, {11, 1}, {16, 0}, {29, 13}}, ink)
	poly([][2]int{{14, 21}, {14, 5}, {25, 14}}, gold)
	poly([][2]int{{16, 17}, {16, 8}, {23, 15}}, pink)
	poly([][2]int{{42, 13}, {55, 0}, {60, 2}, {59, 25}}, ink)
	poly([][2]int{{46, 15}, {56, 5}, {57, 21}}, gold)
	poly([][2]int{{49, 16}, {55, 9}, {55, 19}}, pink)
	headBob := 0
	if phase == "working" || phase == "coding" {
		headBob = t / 3 % 2
	}
	if idle {
		headBob = t / 6
	}
	ellipse(8, 11+headBob, 54, 33, ink)
	ellipse(11, 13+headBob, 48, 28, fur)
	ellipse(16, 13+headBob, 38, 22, gold)
	px(29, 13, 3, 6, shade)
	px(35, 13, 3, 5, shade)
	px(41, 14, 2, 4, shade)
	px(12, 24, 6, 3, shade)
	px(13, 31, 5, 2, shade)
	px(53, 24, 6, 3, shade)
	px(53, 31, 5, 2, shade)
	// A white muzzle, glossy eyes and cheek highlights.
	ellipse(24, 30+headBob, 25, 13, white)
	poly([][2]int{{30, 33}, {36, 25}, {43, 33}}, white)
	sleepy := idle && (idleCycle == 7 || idleCycle == 8 || idleCycle == 27 || stretch)
	happy := phase == "done" || phase == "petting"
	for _, x := range []int{21, 44} {
		if happy {
			px(x, 26, 2, 2, ink)
			px(x+2, 24, 3, 2, ink)
			px(x+5, 26, 2, 2, ink)
		} else if sleepy {
			px(x, 27, 8, 2, ink)
		} else {
			look := 0
			if phase == "hover-left" || (idle && idleCycle >= 24 && idleCycle < 30) {
				look = -2
			}
			if phase == "hover-right" {
				look = 1
			}
			ellipse(x-1, 20+headBob, 11, 14, ink)
			ellipse(x, 20+headBob, 9, 12, white)
			ellipse(x+2+look, 22+headBob, 7, 10, ink)
			px(x+2+look, 22+headBob, 3, 4, white)
			px(x+6+look, 29+headBob, 2, 2, white)
		}
	}
	px(34, 33+headBob, 5, 2, pink)
	px(35, 35+headBob, 3, 1, ink)
	px(32, 37+headBob, 3, 1, ink)
	px(38, 37+headBob, 3, 1, ink)
	px(17, 34, 4, 2, pink)
	px(52, 34, 4, 2, pink)
	px(8, 33, 10, 1, ink)
	px(7, 36, 11, 1, ink)
	px(56, 33, 9, 1, ink)
	px(56, 36, 10, 1, ink)
	px(23, 42, 27, 3, mint)
	px(43, 45, 5, 10, mint)
	px(44, 46, 2, 7, color.RGBA{127, 205, 185, 255})
	if stretch {
		ellipse(9, 32-t%3, 10, 21, ink)
		ellipse(10, 33-t%3, 8, 18, white)
		ellipse(53, 32-t%3, 10, 21, ink)
		ellipse(54, 33-t%3, 8, 18, white)
	}
	if wave || phase == "hover" || phase == "hover-left" || phase == "hover-right" {
		lift := t % 3
		ellipse(7, 37-lift, 11, 16, ink)
		ellipse(8, 38-lift, 9, 13, white)
		px(10, 39-lift, 2, 3, pink)
		px(14, 39-lift, 2, 3, pink)
	}
	working := phase == "working" || phase == "coding" || phase == "thinking" || phase == "reading" || phase == "searching" || phase == "running" || phase == "parallel"
	if working {
		// Angled laptop with screen, bevels, visible keyboard and alternating paws.
		poly([][2]int{{30, 46}, {64, 41}, {66, 57}, {35, 63}}, ink)
		poly([][2]int{{33, 48}, {62, 44}, {63, 55}, {36, 60}}, color.RGBA{96, 126, 151, 255})
		px(43, 50, 8, 1, mint)
		px(40, 53, 10, 1, cream)
		px(44, 56, 10, 1, mint)
		poly([][2]int{{13, 60}, {34, 58}, {63, 59}, {48, 67}, {16, 67}}, ink)
		poly([][2]int{{17, 60}, {36, 60}, {57, 60}, {47, 64}, {18, 64}}, color.RGBA{163, 183, 193, 255})
		for x := 20; x < 47; x += 5 {
			px(x, 61, 3, 1, cream)
		}
		lift := []int{0, 1, 2, 1, 0, 0, 0, 0, 1, 2, 1, 0}[t]
		ellipse(19, 52-lift, 10, 9, ink)
		ellipse(20, 52-lift, 9, 7, white)
		ellipse(32, 54-(2-lift), 10, 8, ink)
		ellipse(33, 54-(2-lift), 8, 6, white)
		if phase == "reading" {
			px(1, 35, 12, 18, ink)
			px(2, 36, 10, 16, white)
			for y := 39; y < 49; y += 4 {
				px(4, y, 6, 1, mint)
			}
		}
		if phase == "searching" {
			ellipse(2, 34, 12, 12, ink)
			ellipse(4, 36, 8, 8, cream)
			px(11, 45, 3, 7, ink)
		}
		if phase == "thinking" {
			px(61, 8, 3, 3, mint)
			px(65, 3, 4, 4, mint)
			px(66, 0, 1, 1, mint)
		}
		if phase == "running" {
			px(61, 12, 4, 2, mint)
			px(63, 10, 2, 6, mint)
		}
	}
	if phase == "waiting" {
		ellipse(6, 41+t/3%2, 10, 8, ink)
		ellipse(7, 41+t/3%2, 8, 6, white)
		px(64, 12, 2, 7, gold)
		px(64, 21, 2, 2, gold)
	}
	if phase == "error" {
		px(23, 18, 7, 2, ink)
		px(43, 18, 7, 2, ink)
		px(63, 12, 2, 8, pink)
		px(63, 23, 2, 2, pink)
	}
	if phase == "dragging" {
		ellipse(8, 42, 9, 9, ink)
		ellipse(9, 43, 7, 7, white)
		ellipse(53, 42, 9, 9, ink)
		ellipse(54, 43, 7, 7, white)
	}
	if happy {
		for _, p := range [][2]int{{6, 15}, {61, 28}, {18, 4}} {
			dy := t % 4
			px(p[0], p[1]-dy, 1, 7, gold)
			px(p[0]-3, p[1]+3-dy, 7, 1, gold)
		}
		if phase == "petting" {
			px(31, 0, 3, 2, pink)
			px(36, 0, 3, 2, pink)
			px(32, 2, 6, 3, pink)
			px(34, 5, 2, 2, pink)
		}
	}
}
