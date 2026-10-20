package pet

import (
	"bytes"
	"testing"
)

func TestFramesAreSmallTransparentAndDistinct(t *testing.T) {
	var previous []byte
	for _, phase := range []string{"idle", "working", "waiting", "done", "error"} {
		frame := Frame(phase, 0, false)
		for i := 0; i < len(frame.Pix); i += 4 {
			if frame.Pix[i] > frame.Pix[i+3] || frame.Pix[i+1] > frame.Pix[i+3] || frame.Pix[i+2] > frame.Pix[i+3] {
				t.Fatal("invalid premultiplied alpha", phase)
			}
		}
		if len(frame.Pix) != Width*Height*4 || frame.RGBAAt(0, 0).A != 0 {
			t.Fatal("oversized or opaque frame")
		}
		if previous != nil && bytes.Equal(previous, frame.Pix) {
			t.Fatal("indistinguishable state", phase)
		}
		previous = frame.Pix
	}
	if bytes.Equal(Frame("working", 0, false).Pix, Frame("working", 1, false).Pix) {
		t.Fatal("work animation missing")
	}
	if Frame("idle", 0, true).RGBAAt(20, 20).A != 255 {
		t.Fatal("bubble not rendered")
	}
}

func TestHoverControlsStayHiddenAndBubbleHasSoftEdges(t *testing.T) {
	frame := Frame("idle", 0, true)
	if frame.RGBAAt(110, ControlsTop+15).A != 0 {
		t.Fatal("idle toolbar should be hidden")
	}
	soft := false
	for y := 8; y < 80; y++ {
		for x := 10; x < 278; x++ {
			pixel := frame.RGBAAt(x, y)
			if pixel.A > 0 && pixel.A < 255 {
				soft = true
			}
		}
	}
	if !soft {
		t.Fatal("bubble edges are not anti-aliased")
	}
	DrawCompanion(frame, "hover", 0, Overlay{Controls: true})
	if frame.RGBAAt(110, ControlsTop+15).A == 0 {
		t.Fatal("hover actions missing")
	}
}
