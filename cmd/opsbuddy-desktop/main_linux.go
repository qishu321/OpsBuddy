//go:build linux

package main

import (
	"crypto/sha256"
	"embed"
	"flag"
	"fmt"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"opsbuddy/internal/desktop"
	"os"
	"path/filepath"
)

//go:embed all:frontend
var frontend embed.FS

func main() {
	config, err := os.UserConfigDir()
	if err != nil {
		fatal(err)
	}
	data := flag.String("data", filepath.Join(config, "opsbuddy"), "local workspace directory")
	smoke := flag.Bool("smoke-test", false, "run hidden integration checks then exit")
	flag.Parse()
	absolute, err := filepath.Abs(*data)
	if err != nil {
		fatal(err)
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		fatal(err)
	}
	controller := desktop.New(absolute)
	if *smoke {
		controller.EnableSmokeTest()
	}
	hash := sha256.Sum256([]byte(absolute))
	width, height := desktop.WindowSize(absolute)
	err = wails.Run(&options.App{
		StartHidden: *smoke, Title: "OpsBuddy", Width: width, Height: height, MinWidth: 880, MinHeight: 600,
		Logger: logger.NewFileLogger(filepath.Join(absolute, "desktop.log")), LogLevelProduction: logger.ERROR,
		BackgroundColour: options.NewRGB(247, 249, 252),
		AssetServer:      &assetserver.Options{Assets: frontend, Handler: controller},
		OnStartup:        controller.Startup, OnShutdown: controller.Shutdown, OnBeforeClose: controller.BeforeClose,
		Bind:               []interface{}{desktop.Bindings(controller)},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("opsbuddy-%x", hash[:8]), OnSecondInstanceLaunch: func(options.SecondInstanceData) { controller.Focus() }},
		Linux:              &linux.Options{ProgramName: "opsbuddy", WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand},
		DragAndDrop:        &options.DragAndDrop{DisableWebViewDrop: false},
	})
	if err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "OpsBuddy:", err); os.Exit(1) }
