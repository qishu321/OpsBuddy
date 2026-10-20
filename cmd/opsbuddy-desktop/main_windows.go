//go:build windows

package main

import (
	"crypto/sha256"
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"opsbuddy/internal/desktop"
)

//go:embed all:frontend
var frontend embed.FS

func main() {
	exe, err := os.Executable()
	if err != nil {
		fatal(err)
		return
	}
	data := flag.String("data", filepath.Join(filepath.Dir(exe), ".opsbuddy"), "local workspace directory")
	smoke := flag.Bool("smoke-test", false, "run embedded desktop integration checks then exit")
	flag.Parse()
	absolute, err := filepath.Abs(*data)
	if err != nil {
		fatal(err)
		return
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		fatal(fmt.Errorf("数据目录不可写，请将程序放在可写文件夹：%w", err))
		return
	}
	controller := desktop.New(absolute)
	if *smoke {
		controller.EnableSmokeTest()
	}
	lockHash := sha256.Sum256([]byte(absolute))
	width, height := desktop.WindowSize(absolute)
	err = wails.Run(&options.App{
		StartHidden: *smoke,
		Title:       "OpsBuddy", Width: width, Height: height, MinWidth: 880, MinHeight: 600,
		Logger: logger.NewFileLogger(filepath.Join(absolute, "desktop.log")), LogLevelProduction: logger.ERROR,
		BackgroundColour: options.NewRGB(247, 249, 252),
		AssetServer:      &assetserver.Options{Assets: frontend, Handler: controller},
		OnStartup:        controller.Startup, OnShutdown: controller.Shutdown, OnBeforeClose: controller.BeforeClose,
		Bind:               []interface{}{desktop.Bindings(controller)},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("opsbuddy-%x", lockHash[:8]), OnSecondInstanceLaunch: func(options.SecondInstanceData) { controller.Focus() }},
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(absolute, "webview"), Theme: windows.SystemDefault, DisablePinchZoom: true},
		DragAndDrop:        &options.DragAndDrop{DisableWebViewDrop: false},
	})
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	title, _ := syscall.UTF16PtrFromString("OpsBuddy")
	message, _ := syscall.UTF16PtrFromString(err.Error())
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
