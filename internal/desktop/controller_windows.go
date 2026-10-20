//go:build windows

package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"opsbuddy/internal/app"
	"opsbuddy/internal/pet"
)

type Controller struct {
	mu        sync.RWMutex
	ctx       context.Context
	core      *app.App
	handler   http.Handler
	dataRoot  string
	ready     chan struct{}
	smoke     bool
	pet       *pet.Companion
	petError  error
	petCancel func()
	petDone   chan struct{}
}

func New(dataRoot string) *Controller {
	return &Controller{dataRoot: dataRoot, ready: make(chan struct{})}
}
func (c *Controller) EnableSmokeTest() { c.smoke = true }

func (c *Controller) Startup(ctx context.Context) {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	defer close(c.ready)
	core, err := app.New(c.dataRoot, "")
	if err != nil {
		wr.MessageDialog(ctx, wr.MessageDialogOptions{Type: wr.ErrorDialog, Title: "OpsBuddy 启动失败", Message: err.Error()})
		wr.Quit(ctx)
		return
	}
	c.mu.Lock()
	c.core = core
	c.handler = core.Handler()
	c.mu.Unlock()
	companion, petErr := pet.New(filepath.Join(c.dataRoot, "pet.json"), c.Focus)
	c.mu.Lock()
	c.pet, c.petError = companion, petErr
	c.mu.Unlock()
	if companion != nil {
		activity, cancel := core.SubscribeActivity()
		c.petCancel, c.petDone = cancel, make(chan struct{})
		go func() {
			defer close(c.petDone)
			for event := range activity {
				companion.ActivityDetail(event.Phase, event.Count, event.Stage)
			}
		}()
	}
	wr.WindowSetTitle(ctx, "OpsBuddy")
}

func (c *Controller) Shutdown(ctx context.Context) {
	if c.petCancel != nil {
		c.petCancel()
		<-c.petDone
	}
	if c.pet != nil {
		c.pet.Close()
	}
	c.mu.RLock()
	core := c.core
	c.mu.RUnlock()
	if core != nil {
		core.Close()
	}
}

func (c *Controller) PetSettings() (pet.Settings, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.petError != nil {
		return pet.Settings{}, c.petError
	}
	if c.pet == nil {
		return pet.Settings{}, errors.New("桌宠尚未就绪")
	}
	return c.pet.Settings(), nil
}
func (c *Controller) ConfigurePet(settings pet.Settings) (pet.Settings, error) {
	c.mu.RLock()
	companion := c.pet
	c.mu.RUnlock()
	if companion == nil {
		return pet.Settings{}, errors.New("桌宠尚未就绪")
	}
	if err := companion.Configure(settings); err != nil {
		return companion.Settings(), err
	}
	return companion.Settings(), nil
}
func (c *Controller) PetDiagnostics() pet.Diagnostics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.pet == nil {
		return pet.Diagnostics{}
	}
	return c.pet.Diagnostics()
}

func (c *Controller) BeforeClose(ctx context.Context) bool {
	width, height := wr.WindowGetSize(ctx)
	if width >= 880 && height >= 600 {
		data, _ := json.Marshal(map[string]int{"width": width, "height": height})
		_ = os.WriteFile(filepath.Join(c.dataRoot, "window.json"), data, 0600)
	}
	c.mu.RLock()
	core := c.core
	c.mu.RUnlock()
	if core == nil || core.ActiveTaskCount() == 0 {
		return false
	}
	answer, err := wr.MessageDialog(ctx, wr.MessageDialogOptions{Type: wr.QuestionDialog, Title: "退出 OpsBuddy", Message: "任务正在运行。退出将停止任务，已生成的文件会保留。是否退出？", Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No"})
	return err != nil || answer != "Yes"
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	// Wails handles this in-process; no TCP listener is created. Keep the browser
	// server's Host/Origin checks intact instead of broadening its trusted origins.
	if r.Host != "wails.localhost" && r.Host != "wails" {
		http.Error(w, "不允许的桌面来源", 403)
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://wails.localhost" && origin != "wails://wails" {
		http.Error(w, "不允许的桌面来源", 403)
		return
	}
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()
	if handler == nil {
		http.Error(w, "桌面服务正在初始化", 503)
		return
	}
	clone := r.Clone(r.Context())
	clone.Host = "127.0.0.1"
	clone.URL.Scheme = "http"
	clone.URL.Host = "127.0.0.1"
	clone.Header.Del("Origin")
	handler.ServeHTTP(w, clone)
}

func (c *Controller) engine() (*app.App, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.core == nil {
		return nil, errors.New("工作空间尚未就绪")
	}
	return c.core, nil
}

func (c *Controller) PickFiles(projectID string) ([]app.Asset, error) {
	core, err := c.engine()
	if err != nil {
		return nil, err
	}
	paths, err := wr.OpenMultipleFilesDialog(c.ctx, wr.OpenDialogOptions{Title: "添加任务材料", Filters: []wr.FileFilter{{DisplayName: "办公文件、代码与图片", Pattern: "*.docx;*.pptx;*.xlsx;*.csv;*.tsv;*.pdf;*.txt;*.md;*.html;*.css;*.js;*.mjs;*.cjs;*.go;*.mod;*.py;*.json;*.png;*.jpg;*.jpeg"}, {DisplayName: "所有文件", Pattern: "*.*"}}})
	if err != nil {
		return nil, err
	}
	return core.ImportLocalFiles(projectID, paths)
}

func (c *Controller) PickFolder(projectID string) ([]app.Asset, error) {
	core, err := c.engine()
	if err != nil {
		return nil, err
	}
	folder, err := wr.OpenDirectoryDialog(c.ctx, wr.OpenDialogOptions{Title: "引用材料文件夹（最多 16 个文件，32 MB）"})
	if err != nil {
		return nil, err
	}
	if folder == "" {
		return []app.Asset{}, nil
	}
	return core.ImportLocalFolder(projectID, folder)
}

func (c *Controller) PickScheduleDirectory(projectID string) (app.DirectoryGrant, error) {
	core, err := c.engine()
	if err != nil {
		return app.DirectoryGrant{}, err
	}
	folder, err := wr.OpenDirectoryDialog(c.ctx, wr.OpenDialogOptions{Title: "授权定时任务工作目录（每次读取最多 16 个文件、32 MB）"})
	if err != nil || folder == "" {
		return app.DirectoryGrant{}, err
	}
	return core.RegisterDirectory(projectID, folder)
}

func (c *Controller) SaveAsset(assetID string) (string, error) {
	core, err := c.engine()
	if err != nil {
		return "", err
	}
	v, _, err := core.LocalAsset(assetID)
	if err != nil {
		return "", err
	}
	ext := v.Kind
	if ext == "code" {
		ext = "zip"
	}
	name := v.Name
	if v.Kind == "code" {
		name = strings.TrimSuffix(name, ".zip") + ".zip"
	}
	target, err := wr.SaveFileDialog(c.ctx, wr.SaveDialogOptions{Title: "另存成果", DefaultFilename: name, Filters: []wr.FileFilter{{DisplayName: strings.ToUpper(ext) + " 文件", Pattern: "*." + ext}}, CanCreateDirectories: true})
	if err != nil || target == "" {
		return "", err
	}
	if err = core.ExportAssetTo(assetID, target); err != nil {
		return "", err
	}
	return target, nil
}

func (c *Controller) RevealAsset(assetID string) error {
	core, err := c.engine()
	if err != nil {
		return err
	}
	_, path, err := core.LocalAsset(assetID)
	if err != nil {
		return err
	}
	return shellOpen("explorer.exe", "/select,\""+path+"\"")
}

func (c *Controller) OpenAsset(assetID string) error {
	core, err := c.engine()
	if err != nil {
		return err
	}
	v, path, err := core.LocalAsset(assetID)
	if err != nil {
		return err
	}
	switch v.Kind {
	case "code":
		folder, err := core.CodeSourceDirectory(assetID)
		if err != nil {
			return err
		}
		return shellOpen(folder, "")
	case "docx", "pptx", "xlsx", "pdf", "txt", "md", "html", "png", "jpg", "jpeg", "zip":
		return shellOpen(path, "")
	default:
		return errors.New("该文件类型不支持直接打开，请另存后自行查看")
	}
}

func (c *Controller) OpenDataFolder() error { return shellOpen(c.dataRoot, "") }
func (c *Controller) ImportProject() (app.ProjectImportResult, error) {
	path, err := wr.OpenFileDialog(c.ctx, wr.OpenDialogOptions{Title: "导入代码协作项目", Filters: []wr.FileFilter{{DisplayName: "OpsBuddy 代码协作项目", Pattern: "*.opsbuddy-project.json"}}})
	if err != nil || path == "" {
		return app.ProjectImportResult{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return app.ProjectImportResult{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return app.ProjectImportResult{}, err
	}
	return c.core.ImportProjectBundle(raw)
}
func (c *Controller) Info() map[string]string {
	smoke := "false"
	if c.smoke {
		smoke = "true"
	}
	info := map[string]string{"mode": "desktop", "version": "0.17.1", "dataDirectory": c.dataRoot, "smokeTest": smoke}
	if c.smoke && os.Getenv("OPSBUDDY_SMOKE_IMPORT_DEMO") == "1" {
		info["smokeImportDemo"] = "true"
	}
	if c.smoke {
		// Explicit test-only local endpoint. Normal desktop launches never expose it.
		if endpoint := os.Getenv("OPSBUDDY_SMOKE_MODEL_URL"); strings.HasPrefix(endpoint, "http://127.0.0.1:") {
			info["smokeModelURL"] = endpoint
		}
	}
	return info
}
func (c *Controller) Focus() {
	c.mu.RLock()
	ctx := c.ctx
	c.mu.RUnlock()
	if ctx != nil {
		wr.WindowUnminimise(ctx)
		wr.WindowShow(ctx)
	}
}

func (c *Controller) Ready() error {
	select {
	case <-c.ready:
	case <-time.After(15 * time.Second):
		return errors.New("桌面工作空间初始化超时")
	}
	_, err := c.engine()
	return err
}

func (c *Controller) ReportSmoke(jsonReport string) error {
	if !c.smoke {
		return errors.New("应用未处于自检模式")
	}
	if !json.Valid([]byte(jsonReport)) || len(jsonReport) > 16000 {
		return errors.New("自检记录格式错误")
	}
	if err := os.WriteFile(filepath.Join(c.dataRoot, "smoke-result.json"), []byte(jsonReport), 0600); err != nil {
		return err
	}
	wr.Quit(c.ctx)
	return nil
}

func shellOpen(path, arguments string) error {
	operation, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	args, err := syscall.UTF16PtrFromString(arguments)
	if err != nil {
		return err
	}
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(operation)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 1)
	if result <= 32 {
		return errors.New("Windows 无法打开该文件，请检查关联程序是否已安装")
	}
	return nil
}

func WindowSize(dataRoot string) (int, int) {
	b, err := os.ReadFile(filepath.Join(dataRoot, "window.json"))
	if err != nil {
		return 1280, 840
	}
	var size struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if json.Unmarshal(b, &size) != nil || size.Width < 880 || size.Width > 3000 || size.Height < 600 || size.Height > 2200 {
		return 1280, 840
	}
	return size.Width, size.Height
}
