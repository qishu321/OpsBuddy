//go:build windows

package pet

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type Companion struct {
	op        sync.Mutex
	mu        sync.Mutex
	path      string
	config    Settings
	run       *nativeRun
	open      func()
	lastPhase string
	lastCount int
	lastStage string
}

func New(path string, open func()) (*Companion, error) {
	config := Settings{Enabled: true, Animate: true, Topmost: true}
	if data, err := os.ReadFile(path); err == nil && len(data) < 4096 {
		loaded := config
		if json.Unmarshal(data, &loaded) == nil {
			config = loaded
		}
	}
	c := &Companion{path: path, config: config, open: open, lastPhase: "idle"}
	if err := c.Configure(config); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Companion) Settings() Settings { c.mu.Lock(); defer c.mu.Unlock(); return c.config }
func (c *Companion) saveLocked() error {
	data, err := json.Marshal(c.config)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(c.path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(c.path+".tmp", c.path)
}
func (c *Companion) Configure(next Settings) error {
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	// Position is owned by dragging, not stale UI settings.
	next.X, next.Y, next.Positioned = c.config.X, c.config.Y, c.config.Positioned
	before := c.config
	c.config = next
	if err := c.saveLocked(); err != nil {
		c.config = before
		c.mu.Unlock()
		return err
	}
	run := c.run
	c.mu.Unlock()
	if !next.Enabled {
		if run != nil {
			postMessage.Call(run.hwnd, 0x10, 0, 0)
			<-run.done
			c.mu.Lock()
			c.run = nil
			c.mu.Unlock()
		}
		return nil
	}
	if run == nil {
		run = &nativeRun{owner: c, started: make(chan error, 1), done: make(chan struct{}), phase: "idle"}
		go run.loop()
		if err := <-run.started; err != nil {
			<-run.done
			return err
		}
		c.mu.Lock()
		c.run = run
		run.mu.Lock()
		run.phase, run.count = c.lastPhase, c.lastCount
		run.stage = c.lastStage
		run.mu.Unlock()
		postMessage.Call(run.hwnd, petActivity, 0, 0)
		c.mu.Unlock()
	} else {
		postMessage.Call(run.hwnd, petConfigure, 0, 0)
	}
	return nil
}
func (c *Companion) Close() {
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	run := c.run
	c.run = nil
	c.mu.Unlock()
	if run != nil {
		postMessage.Call(run.hwnd, 0x10, 0, 0)
		<-run.done
	}
}
func (c *Companion) Activity(phase string, count int) {
	c.ActivityDetail(phase, count, "")
}
func (c *Companion) ActivityDetail(phase string, count int, stage string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPhase, c.lastCount = phase, count
	c.lastStage = stage
	if c.run == nil {
		return
	}
	r := c.run
	r.mu.Lock()
	changed := r.phase != phase || r.count != count || r.stage != stage
	r.phase, r.count = phase, count
	r.stage = stage
	r.mu.Unlock()
	if changed {
		postMessage.Call(r.hwnd, petActivity, 0, 0)
	}
}
func (c *Companion) Diagnostics() Diagnostics {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.run == nil {
		return Diagnostics{}
	}
	r := c.run
	r.mu.Lock()
	phase, stage := r.phase, r.stage
	r.mu.Unlock()
	return Diagnostics{Visible: true, TimerActive: r.timer.Load(), Renders: r.renders.Load(), NativeWindow: true, Phase: phase, Stage: stage, Reactions: r.reactions.Load(), DPI: r.dpi.Load(), FontPixels: scaled(16, r.dpi.Load()), ControlsVisible: r.controlsVisible.Load()}
}

var (
	user          = syscall.NewLazyDLL("user32.dll")
	gdi           = syscall.NewLazyDLL("gdi32.dll")
	createWindow  = user.NewProc("CreateWindowExW")
	defWindow     = user.NewProc("DefWindowProcW")
	postMessage   = user.NewProc("PostMessageW")
	destroyWindow = user.NewProc("DestroyWindow")
	setTimer      = user.NewProc("SetTimer")
	killTimer     = user.NewProc("KillTimer")
	setWindowPos  = user.NewProc("SetWindowPos")
	getWindowRect = user.NewProc("GetWindowRect")
	updateLayer   = user.NewProc("UpdateLayeredWindow")
	selectObject  = gdi.NewProc("SelectObject")
	deleteObject  = gdi.NewProc("DeleteObject")
	windows       sync.Map
	classOnce     sync.Once
	classErr      error
	className     = syscall.StringToUTF16Ptr("OpsBuddyLightPet")
	windowProc    uintptr
)

const petActivity = 0x8001
const petConfigure = 0x8002

type point struct{ X, Y int32 }
type rectangle struct{ Left, Top, Right, Bottom int32 }
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Class                        *uint16
	SmallIcon                          uintptr
}
type message struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type bitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, Bits           uint16
	Compression, ImageSize uint32
	XPels, YPels           int32
	Used, Important        uint32
}
type monitorInfo struct {
	Size          uint32
	Monitor, Work rectangle
	Flags         uint32
}
type nativeRun struct {
	owner                                        *Companion
	mu                                           sync.Mutex
	paintMu                                      sync.Mutex
	phase                                        string
	count                                        int
	stage                                        string
	hwnd, dc, bitmap, oldBitmap, font, titleFont uintptr
	bits                                         unsafe.Pointer
	started                                      chan error
	done                                         chan struct{}
	timer                                        atomic.Bool
	renders                                      atomic.Uint64
	frame                                        int
	bubble                                       bool
	loopRunning                                  bool
	canvas                                       *image.RGBA
	until                                        time.Time
	pose                                         string
	poseUntil                                    time.Time
	hovered, pressed, dragged                    bool
	dragOrigin, dragWindow                       point
	reactions                                    atomic.Uint64
	dpi                                          atomic.Uint32
	width, height                                int
	interval                                     uintptr
	actionPressed                                int
	controlsUntil                                time.Time
	controlsVisible                              atomic.Bool
}

func scaled(value int, dpi uint32) int { return (value*int(max(uint32(96), dpi)) + 48) / 96 }
func controlAt(x, y int) int {
	if y < ControlsTop || y >= ControlsTop+ControlsHeight || x < ControlsLeft || x >= ControlsRight {
		return 0
	}
	if x < Width/2 {
		return 1
	}
	return 2
}
func (r *nativeRun) hitControl(l uintptr) int {
	if !r.controlsVisible.Load() {
		return 0
	}
	x, y := int(int16(l&0xffff)), int(int16((l>>16)&0xffff))
	return controlAt(x*Width/max(1, r.width), y*Height/max(1, r.height))
}

func petWindowProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	value, ok := windows.Load(hwnd)
	if !ok {
		v, _, _ := defWindow.Call(hwnd, uintptr(msg), w, l)
		return v
	}
	r := value.(*nativeRun)
	switch msg {
	case petActivity:
		r.burst()
		return 0
	case petConfigure:
		r.configure()
		r.burst()
		return 0
	case 0x113:
		r.tick()
		return 0
	case 0x2E0: // WM_DPICHANGED: redraw text at the destination monitor's scale.
		if r.dc != 0 {
			if err := r.resizeSurface(uint32(w & 0xffff)); err != nil {
				return 0
			}
			if l != 0 {
				var proposed rectangle
				if ok, _, _ := user.NewProc("CopyRect").Call(uintptr(unsafe.Pointer(&proposed)), l); ok != 0 {
					setWindowPos.Call(r.hwnd, 0, uintptr(proposed.Left), uintptr(proposed.Top), uintptr(r.width), uintptr(r.height), 0x14)
				}
			}
			r.clamp(false)
			r.burst()
		}
		return 0
	case 0x201:
		if action := r.hitControl(l); action != 0 {
			r.actionPressed = action
			user.NewProc("SetCapture").Call(hwnd)
			return 0
		}
		user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&r.dragOrigin)))
		var bounds rectangle
		getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
		r.dragWindow = point{bounds.Left, bounds.Top}
		r.pressed = true
		r.dragged = false
		user.NewProc("SetCapture").Call(hwnd)
		return 0
	case 0x200:
		if r.actionPressed != 0 {
			return 0
		}
		if r.pressed {
			var cursor point
			user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
			dx, dy := cursor.X-r.dragOrigin.X, cursor.Y-r.dragOrigin.Y
			if r.dragged || dx > 4 || dx < -4 || dy > 4 || dy < -4 {
				if !r.dragged {
					r.dragged = true
					r.react("dragging", 3*time.Second)
				}
				setWindowPos.Call(hwnd, 0, uintptr(r.dragWindow.X+dx), uintptr(r.dragWindow.Y+dy), 0, 0, 0x15)
			}
		} else if !r.hovered {
			r.hovered = true
			r.controlsUntil = time.Now().Add(1800 * time.Millisecond)
			track := struct {
				Size, Flags uint32
				Window      uintptr
				HoverTime   uint32
			}{Flags: 2, Window: hwnd}
			track.Size = uint32(unsafe.Sizeof(track))
			user.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&track)))
			r.react("hover", 1200*time.Millisecond)
		} else if strings.HasPrefix(r.pose, "hover") {
			x := int(int16(l&0xffff)) * Width / max(1, r.width)
			pose := "hover"
			if x < Width/2-24 {
				pose = "hover-left"
			} else if x > Width/2+24 {
				pose = "hover-right"
			}
			if pose != r.pose {
				r.pose = pose
				r.paint()
			}
		}
		return 0
	case 0x202:
		if action := r.actionPressed; action != 0 {
			r.actionPressed = 0
			user.NewProc("ReleaseCapture").Call()
			if r.hitControl(l) == action {
				if action == 1 {
					if r.owner.open != nil {
						go r.owner.open()
					}
				} else {
					config := r.owner.Settings()
					config.Animate = !config.Animate
					go func() { _ = r.owner.Configure(config) }()
				}
			}
			return 0
		}
		pressed, dragged := r.pressed, r.dragged
		r.pressed = false
		r.dragged = false
		user.NewProc("ReleaseCapture").Call()
		if pressed {
			if dragged {
				r.clamp(true)
				r.pose = ""
				r.burst()
			} else {
				r.react("petting", 2*time.Second)
			}
		}
		return 0
	case 0x2A3:
		r.hovered = false
		r.controlsUntil = time.Now().Add(1800 * time.Millisecond)
		if strings.HasPrefix(r.pose, "hover") {
			r.pose = ""
			r.burst()
		}
		return 0
	case 0x215:
		r.actionPressed = 0
		if r.pressed {
			r.pressed = false
			r.dragged = false
			r.pose = ""
			r.burst()
		}
		return 0
	case 0x203:
		r.pressed = false
		user.NewProc("ReleaseCapture").Call()
		r.react("hover", time.Second)
		if r.owner.open != nil {
			go r.owner.open()
		}
		return 0
	case 0x7B:
		r.menu()
		return 0
	case 0x232: // Persist once after dragging, not on every mouse movement.
		r.clamp(true)
		return 0
	case 0x7E, 0x1A:
		r.clamp(false)
		return 0
	case 0x21:
		return 3 // MA_NOACTIVATE: typing focus stays in the current app.
	case 0x10:
		destroyWindow.Call(hwnd)
		return 0
	case 0x2:
		killTimer.Call(hwnd, 1)
		r.timer.Store(false)
		if r.loopRunning {
			user.NewProc("PostQuitMessage").Call(0)
		}
		return 0
	}
	v, _, _ := defWindow.Call(hwnd, uintptr(msg), w, l)
	return v
}
func (r *nativeRun) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The pet owns this OS thread. Keep its monitor scaling independent from
	// the embedding host (including test hosts without an application manifest).
	if proc := user.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	defer close(r.done)
	defer func() {
		r.owner.mu.Lock()
		if r.owner.run == r {
			r.owner.run = nil
		}
		r.owner.mu.Unlock()
	}()
	instance, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	classOnce.Do(func() {
		windowProc = syscall.NewCallback(petWindowProc)
		cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32649)
		wc := windowClass{Style: 8, Proc: windowProc, Instance: instance, Cursor: cursor, Class: className}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if ok, _, err := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); ok == 0 {
			classErr = fmt.Errorf("注册桌宠窗口：%w", err)
		}
	})
	if classErr != nil {
		r.started <- classErr
		return
	}
	hwnd, _, err := createWindow.Call(0x08080080, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("OpsBuddy 小搭子"))), 0x80000000, 0, 0, Width, Height, 0, 0, instance, 0)
	if hwnd == 0 {
		r.started <- fmt.Errorf("创建桌宠：%w", err)
		return
	}
	r.hwnd = hwnd
	windows.Store(hwnd, r)
	defer windows.Delete(hwnd)
	r.dc, _, _ = gdi.NewProc("CreateCompatibleDC").Call(0)
	dpi := uint32(96)
	if proc := user.NewProc("GetDpiForWindow"); proc.Find() == nil {
		value, _, _ := proc.Call(hwnd)
		if value >= 96 {
			dpi = uint32(value)
		}
	}
	if r.dc == 0 || r.resizeSurface(dpi) != nil {
		destroyWindow.Call(hwnd)
		if r.dc != 0 {
			gdi.NewProc("DeleteDC").Call(r.dc)
		}
		r.started <- errors.New("无法创建桌宠绘图缓冲区")
		return
	}
	defer func() {
		selectObject.Call(r.dc, r.oldBitmap)
		deleteObject.Call(r.bitmap)
		if r.font != 0 {
			deleteObject.Call(r.font)
		}
		deleteObject.Call(r.titleFont)
		gdi.NewProc("DeleteDC").Call(r.dc)
	}()
	r.configure()
	r.burst()
	if r.renders.Load() == 0 {
		destroyWindow.Call(hwnd)
		r.started <- errors.New("桌宠透明绘图失败")
		return
	}
	user.NewProc("ShowWindow").Call(hwnd, 4)
	r.started <- nil
	r.loopRunning = true
	var msg message
	for {
		ret, _, _ := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}
func (r *nativeRun) configure() {
	config := r.owner.Settings()
	order := ^uintptr(0)
	if !config.Topmost {
		order = ^uintptr(1)
	}
	setWindowPos.Call(r.hwnd, order, 0, 0, 0, 0, 0x13)
	if config.Positioned {
		setWindowPos.Call(r.hwnd, 0, uintptr(config.X), uintptr(config.Y), 0, 0, 0x15)
	} else {
		var work rectangle
		user.NewProc("SystemParametersInfoW").Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0)
		setWindowPos.Call(r.hwnd, 0, uintptr(work.Right-int32(r.width)-20), uintptr(work.Bottom-int32(r.height)-16), 0, 0, 0x15)
	}
	r.clamp(false)
}

// Only the native surface scales. The original pixel geometry stays small and
// reusable; text is rasterized at the monitor DPI instead of enlarging a bitmap.
func (r *nativeRun) resizeSurface(dpi uint32) error {
	r.paintMu.Lock()
	defer r.paintMu.Unlock()
	dpi = max(uint32(96), min(dpi, uint32(288)))
	width, height := scaled(Width, dpi), scaled(Height, dpi)
	var bits unsafe.Pointer
	info := bitmapInfo{Size: 40, Width: int32(width), Height: -int32(height), Planes: 1, Bits: 32}
	bitmap, _, _ := gdi.NewProc("CreateDIBSection").Call(r.dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	newFont := func(size, weight int) uintptr {
		fontSize := -int32(scaled(size, dpi))
		font, _, _ := gdi.NewProc("CreateFontW").Call(uintptr(fontSize), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Microsoft YaHei UI"))))
		return font
	}
	font, title := newFont(16, 400), newFont(14, 500)
	if bitmap == 0 || font == 0 || title == 0 {
		for _, object := range []uintptr{bitmap, font, title} {
			if object != 0 {
				deleteObject.Call(object)
			}
		}
		return errors.New("无法创建清晰文字绘图缓冲区")
	}
	old, _, _ := selectObject.Call(r.dc, bitmap)
	if r.oldBitmap == 0 {
		r.oldBitmap = old
	} else {
		deleteObject.Call(r.bitmap)
	}
	for _, object := range []uintptr{r.font, r.titleFont} {
		if object != 0 {
			deleteObject.Call(object)
		}
	}
	r.bitmap, r.bits, r.font, r.titleFont = bitmap, bits, font, title
	r.width, r.height = width, height
	r.dpi.Store(dpi)
	setWindowPos.Call(r.hwnd, 0, 0, 0, uintptr(width), uintptr(height), 0x16)
	return nil
}
func (r *nativeRun) clamp(save bool) {
	var bounds rectangle
	getWindowRect.Call(r.hwnd, uintptr(unsafe.Pointer(&bounds)))
	monitor, _, _ := user.NewProc("MonitorFromWindow").Call(r.hwnd, 2)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if ok, _, _ := user.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return
	}
	x, y := max(mi.Work.Left, min(bounds.Left, mi.Work.Right-int32(r.width))), max(mi.Work.Top, min(bounds.Top, mi.Work.Bottom-int32(r.height)))
	setWindowPos.Call(r.hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x15)
	if save {
		r.owner.mu.Lock()
		r.owner.config.X, r.owner.config.Y, r.owner.config.Positioned = x, y, true
		_ = r.owner.saveLocked()
		r.owner.mu.Unlock()
	}
}
func (r *nativeRun) react(pose string, duration time.Duration) {
	r.reactions.Add(1)
	r.pose = pose
	r.poseUntil = time.Now().Add(duration)
	r.burst()
}
func (r *nativeRun) tick() {
	r.frame++
	if r.pose != "" && time.Now().After(r.poseUntil) {
		r.pose = ""
		r.frame = 0
	}
	r.mu.Lock()
	phase := r.phase
	r.mu.Unlock()
	animate := r.owner.Settings().Animate
	if time.Now().After(r.until) {
		if r.bubble && phase != "working" {
			r.bubble = false
			r.frame = 0
		}
		if !animate {
			killTimer.Call(r.hwnd, 1)
			r.timer.Store(false)
			r.interval = 0
		} else if phase != "working" && r.pose == "" && r.interval != 500 {
			// Calm idle loop: two frames per second, no polling or global hooks.
			r.setAnimationTimer(500)
		}
	}
	r.paint()
}
func (r *nativeRun) setAnimationTimer(interval uintptr) {
	killTimer.Call(r.hwnd, 1)
	ok, _, _ := setTimer.Call(r.hwnd, 1, interval, 0)
	r.timer.Store(ok != 0)
	r.interval = interval
}
func (r *nativeRun) burst() {
	killTimer.Call(r.hwnd, 1)
	r.frame = 0
	r.bubble = true
	r.until = time.Now().Add(3 * time.Second)
	interval := uintptr(166)
	if !r.owner.Settings().Animate {
		interval = 3100
	}
	r.setAnimationTimer(interval)
	r.paint()
}
func (r *nativeRun) paint() {
	r.paintMu.Lock()
	defer r.paintMu.Unlock()
	r.mu.Lock()
	phase, count, stage := r.phase, r.count, r.stage
	r.mu.Unlock()
	frame := r.frame
	if !r.owner.Settings().Animate {
		frame = 0
	}
	appearance := phase
	if !r.bubble && phase != "working" && phase != "waiting" {
		appearance = "idle"
	}
	if phase == "working" && stage != "" {
		appearance = stage
		if appearance == "writing" {
			appearance = "coding"
		}
	}
	if r.pose != "" {
		appearance = r.pose
	}
	if r.canvas == nil {
		r.canvas = image.NewRGBA(image.Rect(0, 0, Width, Height))
	}
	controls := r.hovered || time.Now().Before(r.controlsUntil) || r.actionPressed != 0
	r.controlsVisible.Store(controls)
	DrawCompanion(r.canvas, appearance, frame, Overlay{Bubble: r.bubble, Controls: controls, Paused: !r.owner.Settings().Animate})
	pixels := r.canvas
	buffer := unsafe.Slice((*byte)(r.bits), r.width*r.height*4)
	for y := 0; y < r.height; y++ {
		for x := 0; x < r.width; x++ {
			i, j := (y*r.width+x)*4, ((y*Height/r.height)*Width+x*Width/r.width)*4
			buffer[i], buffer[i+1], buffer[i+2], buffer[i+3] = pixels.Pix[j+2], pixels.Pix[j+1], pixels.Pix[j], pixels.Pix[j+3]
		}
	}
	dpi := r.dpi.Load()
	textRect := func(area rectangle) rectangle {
		return rectangle{int32(scaled(int(area.Left), dpi)), int32(scaled(int(area.Top), dpi)), int32(scaled(int(area.Right), dpi)), int32(scaled(int(area.Bottom), dpi))}
	}
	old, _, _ := selectObject.Call(r.dc, r.font)
	gdi.NewProc("SetBkMode").Call(r.dc, 1)
	drawText := func(text string, area rectangle, colour, flags, font uintptr) {
		selectObject.Call(r.dc, font)
		gdi.NewProc("SetTextColor").Call(r.dc, colour)
		area = textRect(area)
		user.NewProc("DrawTextW").Call(r.dc, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))), ^uintptr(0), uintptr(unsafe.Pointer(&area)), flags)
		// GDI text clears alpha; each text rectangle has an opaque background.
		for y := int(area.Top); y < int(area.Bottom); y++ {
			for x := int(area.Left); x < int(area.Right); x++ {
				buffer[(y*r.width+x)*4+3] = 255
			}
		}
	}
	if r.bubble {
		heading, label := petCaption(phase, stage, count, r.pose)
		drawText(heading, rectangle{32, 18, 256, 38}, 0x007c8f52, 0x8824, r.titleFont)
		drawText(label, rectangle{32, 41, 256, 66}, 0x00524534, 0x8824, r.font)
	}
	selectObject.Call(r.dc, old)
	origin := point{}
	dimensions := point{int32(r.width), int32(r.height)}
	blend := [4]byte{0, 0, 255, 1}
	if ok, _, _ := updateLayer.Call(r.hwnd, 0, 0, uintptr(unsafe.Pointer(&dimensions)), r.dc, uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), 2); ok != 0 {
		r.renders.Add(1)
	}
}
func petCaption(phase, stage string, count int, pose string) (string, string) {
	if strings.HasPrefix(pose, "hover") {
		return "小搭子 · 在呢", "单击摸摸，双击打开工作台"
	}
	if pose == "petting" {
		return "小搭子 · 开心", "收到摸摸，继续陪你忙～"
	}
	if pose == "dragging" {
		return "小搭子 · 搬个家", "松开鼠标，记住新位置"
	}
	switch phase {
	case "working":
		titles := map[string]string{"reading": "正在读材料", "thinking": "正在思考", "coding": "正在写代码", "writing": "正在生成文件", "running": "正在启动预览", "searching": "正在检索", "parallel": "多 Agent 协作", "queued": "任务排队中"}
		title := titles[stage]
		if title == "" {
			title = "正在工作"
		}
		return title, fmt.Sprintf("%d 项任务进行中 · 双击查看", count)
	case "waiting":
		return "等你确认", "双击返回工作台处理"
	case "done":
		return "完成啦", "成果已准备好，双击查看"
	case "error":
		return "需要帮忙", "双击查看任务遇到的问题"
	default:
		return "小搭子 · 陪你办公", "单击摸摸，双击打开工作台"
	}
}
func (r *nativeRun) menu() {
	menu, _, _ := user.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user.NewProc("DestroyMenu").Call(menu)
	config := r.owner.Settings()
	for _, item := range []struct {
		id      uintptr
		label   string
		checked bool
	}{{1, "打开 OpsBuddy", false}, {2, "轻量活跃动画", config.Animate}, {3, "保持置顶", config.Topmost}, {4, "隐藏桌宠（可在设置中开启）", false}} {
		flag := uintptr(0)
		if item.checked {
			flag = 8
		}
		user.NewProc("AppendMenuW").Call(menu, flag, item.id, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(item.label))))
	}
	var cursor point
	user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
	user.NewProc("SetForegroundWindow").Call(r.hwnd)
	choice, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x100|2, uintptr(cursor.X), uintptr(cursor.Y), 0, r.hwnd, 0)
	postMessage.Call(r.hwnd, 0, 0, 0)
	switch choice {
	case 1:
		if r.owner.open != nil {
			go r.owner.open()
		}
	case 2:
		config.Animate = !config.Animate
		go func() { _ = r.owner.Configure(config) }()
	case 3:
		config.Topmost = !config.Topmost
		go func() { _ = r.owner.Configure(config) }()
	case 4:
		config.Enabled = false
		go func() { _ = r.owner.Configure(config) }()
	}
}
