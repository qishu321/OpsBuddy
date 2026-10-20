package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CodeRunStatus struct {
	AssetID   string `json:"assetId"`
	Status    string `json:"status"`
	Runtime   string `json:"runtime"`
	Mode      string `json:"mode"`
	URL       string `json:"url,omitempty"`
	Logs      string `json:"logs"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
}
type codeRuntime struct {
	mu       sync.Mutex
	view     CodeRunStatus
	cancel   context.CancelFunc
	done     chan struct{}
	server   *http.Server
	stopTree func()
	stopped  bool
}

func (r *codeRuntime) snapshot() CodeRunStatus { r.mu.Lock(); defer r.mu.Unlock(); return r.view }
func (r *codeRuntime) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.view.Logs += strings.ToValidUTF8(string(p), "�")
	if len(r.view.Logs) > 64000 {
		r.view.Logs = r.view.Logs[len(r.view.Logs)-64000:]
	}
	return len(p), nil
}
func (r *codeRuntime) stop() {
	r.mu.Lock()
	r.stopped = true
	r.view.Status = "stopped"
	r.view.URL = ""
	r.cancel()
	if r.stopTree != nil {
		r.stopTree()
	}
	server := r.server
	r.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
}

func (a *App) codeRuntimeAPI(w http.ResponseWriter, r *http.Request) {
	asset, err := a.asset(r.PathValue("id"))
	if err != nil {
		problem(w, 404, err)
		return
	}
	if asset.Kind != "code" {
		problem(w, 400, errors.New("不是代码项目"))
		return
	}
	if r.Method == "POST" && r.PathValue("action") == "start" {
		view, err := a.startCodeRuntime(asset)
		if err != nil {
			problem(w, 400, err)
			return
		}
		respond(w, 200, view)
		return
	}
	a.codeMu.Lock()
	run := a.codeRuns[asset.ID]
	a.codeMu.Unlock()
	if r.Method == "POST" {
		if r.PathValue("action") != "stop" {
			problem(w, 400, errors.New("未知运行操作"))
			return
		}
		if run != nil {
			run.stop()
		}
	}
	if run == nil {
		respond(w, 200, CodeRunStatus{AssetID: asset.ID, Status: "stopped"})
		return
	}
	respond(w, 200, run.snapshot())
}

func (a *App) startCodeRuntime(asset Asset) (CodeRunStatus, error) {
	p, err := a.loadCodeProject(asset)
	if err != nil {
		return CodeRunStatus{}, err
	}
	if p.Mode == "fragment" {
		return CodeRunStatus{}, errors.New("这是成员候选源码，请在主任务集成并验收后启动")
	}
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	a.mu.RLock()
	closing := a.closing
	a.mu.RUnlock()
	if closing {
		return CodeRunStatus{}, errors.New("应用正在关闭")
	}
	if old := a.codeRuns[asset.ID]; old != nil {
		v := old.snapshot()
		if v.Status == "starting" || v.Status == "running" {
			return v, nil
		}
		select {
		case <-old.done:
		default:
			return v, errors.New("上一轮运行正在退出，请稍后重试")
		}
	}
	active := 0
	for _, run := range a.codeRuns {
		v := run.snapshot()
		if v.Status == "starting" || v.Status == "running" {
			active++
		}
	}
	if active >= 3 {
		return CodeRunStatus{}, errors.New("最多同时运行 3 个代码项目，请先停止不用的项目")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &codeRuntime{view: CodeRunStatus{AssetID: asset.ID, Status: "starting", Runtime: p.Runtime, Mode: p.Mode, StartedAt: now()}, cancel: cancel, done: make(chan struct{})}
	a.codeRuns[asset.ID] = run
	a.codeWorkers.Add(1)
	go func() {
		defer a.codeWorkers.Done()
		defer close(run.done)
		defer cancel()
		a.runCode(ctx, asset, p, run)
	}()
	return run.snapshot(), nil
}

func (a *App) waitCodePreview(ctx context.Context, assetID string) error {
	deadline := time.NewTimer(95 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("项目启动超时，源码已保留，请查看运行日志")
		case <-ticker.C:
			a.codeMu.Lock()
			run := a.codeRuns[assetID]
			a.codeMu.Unlock()
			if run == nil {
				continue
			}
			view := run.snapshot()
			switch view.Status {
			case "running":
				return nil
			case "failed", "exited", "stopped":
				return fmt.Errorf("项目启动未成功：%s；请在右侧查看日志", view.Error)
			}
		}
	}
}

func runtimeEnvironment(work string, port int) []string {
	// Do not propagate model keys, shell secrets or application-specific env.
	env := []string{}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "TMP", "TEMP", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "GOCACHE", "GOPATH", "GOMODCACHE"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return append(env, "PORT="+strconv.Itoa(port), "HOST=127.0.0.1", "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOWORK=off")
}

func (a *App) runCode(ctx context.Context, asset Asset, p CodeProject, run *codeRuntime) {
	fail := func(err error) {
		run.mu.Lock()
		defer run.mu.Unlock()
		if !run.stopped {
			run.view.Status = "failed"
			run.view.Error = err.Error()
			run.view.URL = ""
		}
	}
	base := filepath.Join(a.root, "code-runs")
	if err := os.MkdirAll(base, 0700); err != nil {
		fail(err)
		return
	}
	work, err := os.MkdirTemp(base, asset.ID+"-")
	if err != nil {
		fail(err)
		return
	}
	for _, f := range append(append([]CodeFile{}, p.Files...), bundledProjectFiles(p)...) {
		full := filepath.Join(work, filepath.FromSlash(f.Path))
		if err = os.MkdirAll(filepath.Dir(full), 0700); err == nil {
			err = os.WriteFile(full, []byte(f.Content), 0600)
		}
		if err != nil {
			fail(err)
			return
		}
	}
	port := 0
	if p.Mode == "server" {
		reservation, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			fail(e)
			return
		}
		port = reservation.Addr().(*net.TCPAddr).Port
		_ = reservation.Close()
	}
	var cmd *exec.Cmd
	switch p.Runtime {
	case "go":
		cmd = exec.CommandContext(ctx, "go", "run", "./"+path.Dir(p.Entry))
	case "node":
		cmd = exec.CommandContext(ctx, "node", "./"+p.Entry)
	case "python":
		cmd = exec.CommandContext(ctx, "python", "-u", "./"+p.Entry)
	}
	cmd.Dir = work
	cmd.Env = runtimeEnvironment(work, port)
	cmd.Stdout = run
	cmd.Stderr = run
	cmd.WaitDelay = 3 * time.Second
	prepareCodeProcess(cmd)
	fmt.Fprintf(run, "%s · %s · %s\n工作目录：%s\n", p.Runtime, p.Mode, p.Entry, work)
	if err = cmd.Start(); err != nil {
		fail(fmt.Errorf("无法启动 %s，请安装并加入 PATH：%w", p.Runtime, err))
		return
	}
	stopTree, err := ownCodeProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		fail(err)
		return
	}
	run.mu.Lock()
	run.stopTree = stopTree
	wasStopped := run.stopped
	run.mu.Unlock()
	if wasStopped {
		stopTree()
	}
	defer stopTree()
	processDone := make(chan error, 1)
	go func() { processDone <- cmd.Wait() }()
	if p.Mode == "server" {
		target := "http://127.0.0.1:" + strconv.Itoa(port)
		client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		defer client.CloseIdleConnections()
		ready := false
		deadline := time.NewTimer(90 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for !ready {
			select {
			case e := <-processDone:
				fail(fmt.Errorf("服务就绪前已退出：%v；请查看日志", e))
				return
			case <-ctx.Done():
				stopTree()
				<-processDone
				return
			case <-deadline.C:
				fail(errors.New("服务在 90 秒内未就绪，请检查 PORT/127.0.0.1 配置及日志"))
				stopTree()
				<-processDone
				return
			case <-ticker.C:
				req, _ := http.NewRequestWithContext(ctx, "GET", target+"/", nil)
				resp, e := client.Do(req)
				if e == nil {
					ready = resp.StatusCode >= 200 && resp.StatusCode < 400
					_ = resp.Body.Close()
				}
			}
		}
		if err = a.serveCodePreview(run, target); err != nil {
			fail(err)
			stopTree()
			<-processDone
			return
		}
	} else {
		run.mu.Lock()
		if !run.stopped {
			run.view.Status = "running"
		}
		run.mu.Unlock()
	}
	select {
	case err = <-processDone:
	case <-ctx.Done():
		stopTree()
		err = <-processDone
	}
	run.mu.Lock()
	if !run.stopped {
		run.view.URL = ""
		run.view.Status = "exited"
		code := cmd.ProcessState.ExitCode()
		run.view.ExitCode = &code
		if err != nil {
			run.view.Status = "failed"
			run.view.Error = err.Error()
		}
	}
	server := run.server
	run.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
}

// A separate per-run loopback origin exposes only the generated project's service.
// It cannot route into the desktop API, and its unguessable path is a capability.
func (a *App) serveCodePreview(run *codeRuntime, target string) error {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	base := "http://" + ln.Addr().String()
	prefix := "/" + id() + "/"
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 15 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != ln.Addr().String() || !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Local Vue templates compile only inside the isolated project origin.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval' "+base+prefix+"; style-src 'unsafe-inline' "+base+prefix+"; img-src data: blob: "+base+prefix+"; connect-src "+base+prefix+"; font-src data:; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action "+base+prefix)
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		if !strings.Contains(" GET POST PUT PATCH DELETE HEAD ", " "+r.Method+" ") {
			w.WriteHeader(405)
			return
		}
		suffix := strings.TrimPrefix(r.URL.Path, prefix)
		if strings.Contains(suffix, "..") || strings.ContainsAny(suffix, "\\\x00") {
			w.WriteHeader(400)
			return
		}
		if serveBundledRuntime(w, r, suffix) {
			return
		}
		u := target + "/" + suffix
		if r.URL.RawQuery != "" {
			u += "?" + r.URL.RawQuery
		}
		req, e := http.NewRequestWithContext(r.Context(), r.Method, u, http.MaxBytesReader(w, r.Body, 1<<20))
		if e != nil {
			http.Error(w, "请求无效", 400)
			return
		}
		if v := r.Header.Get("Content-Type"); v != "" {
			req.Header.Set("Content-Type", v)
		}
		resp, e := client.Do(req)
		if e != nil {
			http.Error(w, "项目服务不可用", 502)
			return
		}
		defer resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "" {
			u, e := url.Parse(loc)
			if e != nil || u.IsAbs() || u.Host != "" {
				http.Error(w, "预览不支持外部跳转", 502)
				return
			}
			w.Header().Set("Location", prefix+strings.TrimPrefix(u.String(), "/"))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			if err != nil {
				http.Error(w, "页面读取失败", 502)
				return
			}
			w.WriteHeader(resp.StatusCode)
			_, _ = io.WriteString(w, previewHTML(string(raw)))
			return
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
	})
	run.mu.Lock()
	if run.stopped {
		run.mu.Unlock()
		_ = ln.Close()
		return context.Canceled
	}
	run.server = server
	run.view.URL = base + prefix
	run.view.Status = "running"
	run.mu.Unlock()
	go func() { _ = server.Serve(ln); transport.CloseIdleConnections() }()
	return nil
}

func (a *App) stopCodeRuntimes() {
	a.codeMu.Lock()
	for _, run := range a.codeRuns {
		run.stop()
	}
	a.codeMu.Unlock()
	a.codeWorkers.Wait()
}
