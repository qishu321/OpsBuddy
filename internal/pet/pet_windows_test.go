//go:build windows

package pet

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestDisabledCompanionHasNoWindowAndPersistsPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pet.json")
	if err := os.WriteFile(path, []byte(`{"enabled":false,"animate":false,"topmost":true,"x":45,"y":60,"positioned":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if view := c.Diagnostics(); view.Visible || view.NativeWindow || view.Renders != 0 || view.TimerActive {
		t.Fatal(view)
	}
	c.Activity("working", 3)
	settings := c.Settings()
	settings.Topmost = false
	settings.X = 1000
	if err = c.Configure(settings); err != nil {
		t.Fatal(err)
	}
	other, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if cfg := other.Settings(); cfg.Topmost || cfg.Animate || cfg.Enabled || cfg.X != 45 || cfg.Y != 60 {
		t.Fatal(cfg)
	}
}

func TestPetTextAndActionScale(t *testing.T) {
	for _, test := range []struct {
		dpi    uint32
		pixels int
	}{{96, 16}, {144, 24}, {192, 32}} {
		if actual := scaled(16, test.dpi); actual != test.pixels {
			t.Fatal("font DPI", test, actual)
		}
	}
	if controlAt(120, ControlsTop+18) != 1 || controlAt(200, ControlsTop+18) != 2 || controlAt(120, ControlsTop-1) != 0 {
		t.Fatal("action boundaries")
	}
}

// Opt-in: requires an interactive Windows desktop; run in an isolated data root.
func TestNativeCompanionLifecycle(t *testing.T) {
	if os.Getenv("OPSBUDDY_PET_NATIVE_TEST") != "1" {
		t.Skip("native desktop opt-in")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if proc := user.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3))
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	path := filepath.Join(t.TempDir(), "pet.json")
	if err := os.WriteFile(path, []byte(`{"enabled":false,"animate":true,"topmost":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	var opened atomic.Int32
	c, err := New(path, func() { opened.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	metrics := func() (uint64, uint64, uint64) {
		var creation, exit, kernel, userTime uint64
		syscallKernel := syscall.NewLazyDLL("kernel32.dll")
		syscallKernel.NewProc("GetProcessTimes").Call(^uintptr(0), uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&userTime)))
		var counters struct {
			Size, Faults                                                                                    uint32
			PeakWorking, Working, PeakPaged, Paged, PeakNonPaged, NonPaged, Pagefile, PeakPagefile, Private uintptr
		}
		counters.Size = uint32(unsafe.Sizeof(counters))
		syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(^uintptr(0), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
		gdiCount, _, _ := user.NewProc("GetGuiResources").Call(^uintptr(0), 0)
		return kernel + userTime, uint64(counters.Private), uint64(gdiCount)
	}
	runtime.GC()
	_, baselineMemory, baselineGDI := metrics()
	settings := c.Settings()
	settings.Enabled = true
	if err = c.Configure(settings); err != nil {
		t.Fatal(err)
	}
	// The first frame proves actual native layered-window rendering succeeded.
	if diag := c.Diagnostics(); !diag.NativeWindow || diag.Renders < 1 {
		t.Fatal(diag)
	}
	c.mu.Lock()
	r := c.run
	c.mu.Unlock()
	// Keep this test window away from the user's pointer while measuring idle.
	// Pointer entry is intentionally interactive and raises the animation rate.
	var pointer point
	user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pointer)))
	var work rectangle
	user.NewProc("SystemParametersInfoW").Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0)
	testX := work.Left + 20
	if pointer.X < (work.Left+work.Right)/2 {
		testX = work.Right - int32(r.width) - 20
	}
	setWindowPos.Call(r.hwnd, 0, uintptr(testX), uintptr(work.Top+20), 0, 0, 0x15)
	postMessage.Call(r.hwnd, 0x232, 0, 0)
	for i := 0; i < 40 && !c.Settings().Positioned; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	// Send a valid synchronous DPI message, as Windows includes a suggested rect.
	originalDPI := c.Diagnostics().DPI
	for _, dpi := range []uint32{144, 192, 96, originalDPI} {
		before := c.Diagnostics()
		var proposed rectangle
		getWindowRect.Call(r.hwnd, uintptr(unsafe.Pointer(&proposed)))
		user.NewProc("SendMessageW").Call(r.hwnd, 0x2E0, uintptr(dpi)|uintptr(dpi)<<16, uintptr(unsafe.Pointer(&proposed)))
		if current := c.Diagnostics(); current.DPI != dpi || current.FontPixels != scaled(16, dpi) || current.Renders <= before.Renders {
			t.Fatal("DPI change did not redraw fonts", dpi, current)
		}
	}
	if output := os.Getenv("OPSBUDDY_PET_PREVIEW"); output != "" {
		// Copy the actual native DIB under the same lock used by painting.
		settings.Animate = false
		if err = c.Configure(settings); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		r.paintMu.Lock()
		img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
		buffer := unsafe.Slice((*byte)(r.bits), r.width*r.height*4)
		for i := 0; i < len(buffer); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buffer[i+2], buffer[i+1], buffer[i], buffer[i+3]
		}
		r.paintMu.Unlock()
		background := image.NewRGBA(img.Bounds())
		draw.Draw(background, background.Bounds(), &image.Uniform{C: color.RGBA{237, 242, 248, 255}}, image.Point{}, draw.Src)
		draw.Draw(background, background.Bounds(), img, image.Point{}, draw.Over)
		f, e := os.Create(output)
		if e != nil {
			t.Fatal(e)
		}
		e = png.Encode(f, background)
		_ = f.Close()
		if e != nil {
			t.Fatal(e)
		}
		settings.Animate = true
		if err = c.Configure(settings); err != nil {
			t.Fatal(err)
		}
	}
	// Calm idle remains alive after the greeting, bounded to two frames/second.
	c.Activity("idle", 0)
	time.Sleep(3400 * time.Millisecond)
	idleStart := c.Diagnostics()
	animatedCPUStart, _, _ := metrics()
	time.Sleep(3 * time.Second)
	animatedCPUEnd, _, _ := metrics()
	idleEnd := c.Diagnostics()
	idleFrames := idleEnd.Renders - idleStart.Renders
	if !idleEnd.TimerActive || idleFrames < 4 || idleFrames > 9 || idleEnd.FontPixels < 16 {
		t.Fatal("calm idle or legible font unavailable", idleStart, idleEnd)
	}
	c.Activity("working", 2)
	time.Sleep(4500 * time.Millisecond)
	diag := c.Diagnostics()
	if !diag.TimerActive {
		t.Fatal("working animation stopped while task is running")
	}
	settings.Animate = false
	if err = c.Configure(settings); err != nil {
		t.Fatal(err)
	}
	c.Activity("idle", 0)
	time.Sleep(3400 * time.Millisecond)
	diag = c.Diagnostics()
	if diag.TimerActive {
		t.Fatal("idle animation did not stop")
	}
	runtime.GC()
	beforeCPU, memory, _ := metrics()
	start := time.Now()
	time.Sleep(3 * time.Second)
	afterCPU, _, _ := metrics()
	elapsed := time.Since(start)
	if after := c.Diagnostics(); after.Renders != diag.Renders || after.TimerActive {
		t.Fatal("idle native window repainted", diag, after)
	}
	if c.Diagnostics().ControlsVisible {
		t.Fatal("toolbar is visible without hovering")
	}
	postMessage.Call(r.hwnd, 0x203, 0, 0)
	for i := 0; i < 40 && opened.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if opened.Load() != 1 {
		t.Fatal("double click did not open main app")
	}
	reactions := c.Diagnostics().Reactions
	postMessage.Call(r.hwnd, 0x2A3, 0, 0)
	postMessage.Call(r.hwnd, 0x200, 0, 0)
	postMessage.Call(r.hwnd, 0x201, 0, 0)
	postMessage.Call(r.hwnd, 0x202, 0, 0)
	for i := 0; i < 40 && c.Diagnostics().Reactions < reactions+2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if c.Diagnostics().Reactions < reactions+2 {
		t.Fatal("hover/click did not react")
	}
	if !c.Diagnostics().ControlsVisible {
		t.Fatal("hover did not reveal toolbar")
	}
	clickControl := func(x int) {
		dpi := c.Diagnostics().DPI
		pos := uintptr(scaled(x, dpi)) | uintptr(scaled(ControlsTop+18, dpi))<<16
		postMessage.Call(r.hwnd, 0x201, 0, pos)
		postMessage.Call(r.hwnd, 0x202, 0, pos)
	}
	clickControl(120)
	for i := 0; i < 40 && opened.Load() != 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if opened.Load() != 2 {
		t.Fatal("open action did not return to workspace")
	}
	clickControl(200)
	for i := 0; i < 40 && !c.Settings().Animate; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !c.Settings().Animate {
		t.Fatal("motion action did not resume animation")
	}
	previousPosition := c.Settings()
	setWindowPos.Call(r.hwnd, 0, 999999, 999999, 0, 0, 0x15)
	postMessage.Call(r.hwnd, 0x232, 0, 0)
	for i := 0; i < 40 && c.Settings().X == previousPosition.X && c.Settings().Y == previousPosition.Y; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	position := c.Settings()
	if !position.Positioned || position.X >= 999999 || position.Y >= 999999 || (position.X == previousPosition.X && position.Y == previousPosition.Y) {
		t.Fatal("position was not clamped/persisted", position)
	}
	for _, phase := range []string{"waiting", "done", "error"} {
		c.Activity(phase, 1)
		time.Sleep(50 * time.Millisecond)
		if c.Diagnostics().Phase != phase {
			t.Fatal("state not applied", phase)
		}
	}
	settings.Enabled = false
	if err = c.Configure(settings); err != nil {
		t.Fatal(err)
	}
	if diag = c.Diagnostics(); diag.Visible || diag.TimerActive || diag.NativeWindow {
		t.Fatal("disable left native resources alive", diag)
	}
	_, _, finalGDI := metrics()
	if finalGDI > baselineGDI+1 {
		t.Fatal("GDI resources leaked", baselineGDI, finalGDI)
	}
	for i := 0; i < 3; i++ {
		settings.Enabled = true
		if err = c.Configure(settings); err != nil {
			t.Fatal(err)
		}
		settings.Enabled = false
		if err = c.Configure(settings); err != nil {
			t.Fatal(err)
		}
	}
	_, _, finalGDI = metrics()
	if finalGDI > baselineGDI+1 {
		t.Fatal("repeated enable leaked drawing objects", baselineGDI, finalGDI)
	}
	report := map[string]any{"passed": true, "nativeLayerRendered": true, "idleRepaints": 0, "idleSeconds": elapsed.Seconds(), "idleCPUSeconds": float64(afterCPU-beforeCPU) / 1e7, "idleCPUPercentOneCore": float64(afterCPU-beforeCPU) / 1e7 / elapsed.Seconds() * 100, "privateMemoryBefore": baselineMemory, "privateMemoryWithPet": memory, "gdiBefore": baselineGDI, "gdiAfterDisable": finalGDI, "dragClampedAndSaved": true, "doubleClick": true, "repeatedEnableDisable": true}
	if output := os.Getenv("OPSBUDDY_PET_REPORT"); output != "" {
		report["animatedIdleFramesIn3Seconds"] = idleFrames
		report["animatedIdleCPUSeconds"] = float64(animatedCPUEnd-animatedCPUStart) / 1e7
		report["readableFontPixels"] = idleEnd.FontPixels
		report["actionStrip"] = true
		report["toolbarHiddenUntilHover"] = true
		report["dpiChanges"] = []int{96, 144, 192}
		data, _ := json.MarshalIndent(report, "", "  ")
		if err = os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
