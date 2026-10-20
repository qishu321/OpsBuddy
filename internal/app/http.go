package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"opsbuddy/internal/version"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, e error) {
	respond(w, status, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		problem(w, 400, errors.New("请求格式错误或内容过大"))
		return false
	}
	return true
}
func (a *App) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.snapshot()) })
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"status": "ok", "version": version.Version, "backend": "Go"})
	})
	m.HandleFunc("POST /api/projects", a.createProject)
	m.HandleFunc("PUT /api/projects/{id}", a.updateProject)
	m.HandleFunc("POST /api/assets", a.upload)
	m.HandleFunc("GET /api/assets/{id}/file", a.download)
	m.HandleFunc("GET /api/assets/{id}/text", a.textAsset)
	m.HandleFunc("POST /api/assets/{id}/versions", a.createTextVersion)
	m.HandleFunc("GET /api/assets/{id}/code", a.codeProjectAPI)
	m.HandleFunc("POST /api/code-projects", a.createCodeProjectAPI)
	m.HandleFunc("POST /api/assets/{id}/code", a.codeProjectAPI)
	m.HandleFunc("GET /api/assets/{id}/runtime", a.codeRuntimeAPI)
	m.HandleFunc("POST /api/assets/{id}/runtime/{action}", a.codeRuntimeAPI)
	m.HandleFunc("POST /api/tasks/{id}/html", a.extractChatHTML)
	m.HandleFunc("POST /api/tasks", a.createTask)
	m.HandleFunc("POST /api/memories", a.saveMemory)
	m.HandleFunc("DELETE /api/memories/{id}", a.deleteMemory)
	m.HandleFunc("POST /api/schedules", a.saveSchedule)
	m.HandleFunc("POST /api/schedules/preview", a.previewSchedule)
	m.HandleFunc("POST /api/schedules/{id}/{action}", a.scheduleAction)
	m.HandleFunc("DELETE /api/schedules/{id}", a.deleteSchedule)
	m.HandleFunc("POST /api/schedule-runs/{id}/cancel", a.cancelScheduleRun)
	m.HandleFunc("POST /api/notifications/{id}/read", a.readNotification)
	m.HandleFunc("POST /api/agent-groups", a.dispatchAgents)
	m.HandleFunc("POST /api/agent-groups/{id}/members/{member}/instructions", a.instructAgent)
	m.HandleFunc("POST /api/agent-groups/{id}/cancel", a.stopAgents)
	m.HandleFunc("POST /api/agent-groups/{id}/pause", a.pauseAgents)
	m.HandleFunc("POST /api/agent-groups/{id}/resume", a.resumeAgents)
	m.HandleFunc("POST /api/agent-groups/{id}/plan", a.reviseAgentPlan)
	m.HandleFunc("POST /api/agent-groups/{id}/recovery", a.agentRecoveryOption)
	m.HandleFunc("POST /api/tasks/{id}/cancel", a.cancelTask)
	m.HandleFunc("POST /api/tasks/{id}/archive", a.archiveTask)
	m.HandleFunc("DELETE /api/tasks/{id}", a.deleteConversation)
	m.HandleFunc("POST /api/models", a.saveModel)
	m.HandleFunc("DELETE /api/models/{id}", a.deleteModel)
	m.HandleFunc("POST /api/models/{id}/test", a.testModel)
	m.HandleFunc("POST /api/roles", a.saveRole)
	m.HandleFunc("DELETE /api/roles/{id}", a.deleteRole)
	m.HandleFunc("POST /api/extensions", a.saveExtension)
	m.HandleFunc("DELETE /api/extensions/{id}", a.deleteExtension)
	m.HandleFunc("POST /api/extensions/{id}/test", a.testExtension)
	m.HandleFunc("POST /api/knowledge", a.createKnowledgeBase)
	m.HandleFunc("POST /api/knowledge/{id}/documents", a.indexKnowledgeAssets)
	m.HandleFunc("GET /api/knowledge/documents/{id}", a.knowledgeDocument)
	m.HandleFunc("DELETE /api/knowledge/documents/{id}", a.removeKnowledgeDocument)
	m.HandleFunc("POST /api/knowledge/search", a.searchKnowledge)
	m.HandleFunc("POST /api/workflows", a.saveWorkflow)
	m.HandleFunc("POST /api/workflows/{id}/runs", a.startWorkflow)
	m.HandleFunc("POST /api/workflow-runs/{id}/cancel", a.cancelWorkflow)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(a.web, filepath.Clean("/"+r.URL.Path))
		if info, e := os.Stat(p); e == nil && !info.IsDir() {
			http.FileServer(http.Dir(a.web)).ServeHTTP(w, r)
			return
		}
		if _, e := os.Stat(filepath.Join(a.web, "index.html")); e == nil {
			http.ServeFile(w, r, filepath.Join(a.web, "index.html"))
			return
		}
		http.Error(w, "桌面界面资源缺失，请重新构建并启动 OpsBuddy 桌面版。", 404)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		host := r.Host
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			problem(w, 403, errors.New("仅支持本机访问"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://"+r.Host && origin != "http://127.0.0.1:5173" && origin != "http://localhost:5173" {
				problem(w, 403, errors.New("不允许跨站访问本地服务"))
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-OpsBuddy-Request") != "1" {
				problem(w, 403, errors.New("缺少本地请求校验"))
				return
			}
		}
		m.ServeHTTP(w, r)
	})
}
func (a *App) createProject(w http.ResponseWriter, r *http.Request) {
	var p Project
	if !decode(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 160 {
		problem(w, 400, errors.New("项目名称不能为空且需小于 160 字节"))
		return
	}
	p.ID = id()
	p.CreatedAt = now()
	a.mu.Lock()
	a.state.Projects = append(a.state.Projects, p)
	e := a.saveLocked()
	a.mu.Unlock()
	if e != nil {
		problem(w, 500, e)
		return
	}
	respond(w, 201, p)
}
func safeName(s string) string {
	s = filepath.Base(strings.ReplaceAll(s, "\\", "/"))
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune("<>:\"/\\|?*", r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		s = "untitled"
	}
	rr := []rune(s)
	if len(rr) > 100 {
		s = string(rr[:100])
	}
	return s
}

func (a *App) updateProject(w http.ResponseWriter, r *http.Request) {
	var in Project
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 100 || len([]rune(in.Description)) > 3000 {
		problem(w, 400, errors.New("项目需要名称，说明最多 3000 字"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, p := range a.state.Projects {
		if p.ID == r.PathValue("id") {
			p.Name = in.Name
			p.Description = in.Description
			a.state.Projects[i] = p
			if err := a.saveLocked(); err != nil {
				problem(w, 500, err)
				return
			}
			respond(w, 200, p)
			return
		}
	}
	problem(w, 404, errors.New("项目不存在"))
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if e := r.ParseMultipartForm(4 << 20); e != nil {
		problem(w, 400, errors.New("上传失败：单次最多 32 MB"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	project := r.FormValue("projectId")
	valid := false
	a.mu.RLock()
	for _, p := range a.state.Projects {
		if p.ID == project {
			valid = true
		}
	}
	a.mu.RUnlock()
	if !valid {
		problem(w, 400, errors.New("请选择有效项目"))
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > 16 {
		problem(w, 400, errors.New("每次选择 1–16 个文件"))
		return
	}
	out := []Asset{}
	for _, f := range files {
		src, e := f.Open()
		if e != nil {
			problem(w, 400, e)
			return
		}
		assetID := id()
		name := safeName(f.Filename)
		ext := strings.ToLower(filepath.Ext(name))
		relative := filepath.ToSlash(filepath.Join("files", assetID+ext))
		dst, e := os.OpenFile(filepath.Join(a.root, relative), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			src.Close()
			problem(w, 500, e)
			return
		}
		n, e := io.Copy(dst, src)
		src.Close()
		ce := dst.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			problem(w, 500, e)
			return
		}
		v := Asset{ID: assetID, Name: name, Kind: strings.TrimPrefix(ext, "."), Size: n, ProjectID: project, CreatedAt: now(), Path: relative, Validation: "用户输入，尚未处理"}
		if e = a.addAsset(v); e != nil {
			problem(w, 500, e)
			return
		}
		v.Path = ""
		out = append(out, v)
	}
	respond(w, 201, out)
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	v, e := a.asset(r.PathValue("id"))
	if e != nil {
		problem(w, 404, e)
		return
	}
	if v.Kind == "code" {
		a.downloadCodeProject(w, r, v)
		return
	}
	p, e := a.filePath(v)
	if e != nil {
		problem(w, 404, errors.New("文件已移走或超出授权目录"))
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && (v.Kind == "pdf" || v.Kind == "png" || v.Kind == "jpg" || v.Kind == "jpeg") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": v.Name}))
	http.ServeFile(w, r, p)
}
func validateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("请输入不含凭据、查询参数的有效服务地址")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return errors.New("远程服务必须使用 HTTPS，本地服务可使用 HTTP")
	}
	return nil
}
func (a *App) saveModel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Model
		APIKey string `json:"apiKey"`
	}
	if !decode(w, r, &in) {
		return
	}
	m := in.Model
	m.Name = strings.TrimSpace(m.Name)
	m.BaseURL = strings.TrimRight(strings.TrimSpace(m.BaseURL), "/")
	if m.Name == "" || m.ModelID == "" {
		problem(w, 400, errors.New("请填写名称和模型 ID"))
		return
	}
	if m.Family != "openai" && m.Family != "anthropic" && m.Family != "gemini" {
		problem(w, 400, errors.New("未知模型接口类型"))
		return
	}
	if e := validateURL(m.BaseURL); e != nil {
		problem(w, 400, e)
		return
	}
	m.Secret = ""
	m.TestedAt = ""
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := -1
	for i, v := range a.state.Models {
		if v.ID == m.ID {
			idx = i
			m.Secret = v.Secret
		}
	}
	if in.APIKey != "" {
		s, e := seal(in.APIKey)
		if e != nil {
			problem(w, 500, e)
			return
		}
		m.Secret = s
	}
	if idx < 0 {
		m.ID = id()
		a.state.Models = append(a.state.Models, m)
	} else {
		a.state.Models[idx] = m
	}
	if e := a.saveLocked(); e != nil {
		problem(w, 500, e)
		return
	}
	m.HasKey = m.Secret != ""
	m.Secret = ""
	respond(w, 200, m)
}
func (a *App) deleteModel(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, v := range a.state.Models {
		if v.ID == r.PathValue("id") {
			a.state.Models = append(a.state.Models[:i], a.state.Models[i+1:]...)
			if e := a.saveLocked(); e != nil {
				problem(w, 500, e)
				return
			}
			respond(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, errors.New("模型配置不存在"))
}
func (a *App) testModel(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	var m Model
	for _, v := range a.state.Models {
		if v.ID == r.PathValue("id") {
			m = v
		}
	}
	a.mu.RUnlock()
	if m.ID == "" {
		problem(w, 404, errors.New("模型配置不存在"))
		return
	}
	_, e := generate(r.Context(), m, "你是连接测试助手。", "返回 JSON：{\"title\":\"连接成功\",\"sections\":[{\"heading\":\"测试\",\"body\":\"OK\"}]}")
	if e != nil {
		problem(w, 400, e)
		return
	}
	a.mu.Lock()
	for i := range a.state.Models {
		if a.state.Models[i].ID == m.ID {
			a.state.Models[i].TestedAt = now()
		}
	}
	e = a.saveLocked()
	a.mu.Unlock()
	if e != nil {
		problem(w, 500, e)
		return
	}
	respond(w, 200, map[string]string{"message": "文本与结构化内容解析通过；流式和通用工具调用尚未验证。"})
}
func (a *App) saveRole(w http.ResponseWriter, r *http.Request) {
	var v Role
	if !decode(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Name) == "" {
		problem(w, 400, errors.New("请填写角色名称"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, old := range a.state.Roles {
		if old.ID == v.ID && old.Builtin {
			problem(w, 400, errors.New("内置角色请复制后修改"))
			return
		}
	}
	v.Builtin = false
	if v.Kind == "" {
		v.Kind = "auto"
	}
	idx := -1
	for i, old := range a.state.Roles {
		if old.ID == v.ID {
			idx = i
		}
	}
	if idx < 0 {
		v.ID = id()
		a.state.Roles = append(a.state.Roles, v)
	} else {
		v.MigratedSkillID = a.state.Roles[idx].MigratedSkillID
		a.state.Roles[idx] = v
	}
	if e := a.saveLocked(); e != nil {
		problem(w, 500, e)
		return
	}
	respond(w, 200, v)
}
func (a *App) deleteRole(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, v := range a.state.Roles {
		if v.ID == r.PathValue("id") {
			if v.Builtin {
				problem(w, 400, errors.New("内置角色不能删除"))
				return
			}
			a.state.Roles = append(a.state.Roles[:i], a.state.Roles[i+1:]...)
			if e := a.saveLocked(); e != nil {
				problem(w, 500, e)
				return
			}
			respond(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, errors.New("角色不存在"))
}
func (a *App) saveExtension(w http.ResponseWriter, r *http.Request) {
	var v Extension
	if !decode(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Name) == "" {
		problem(w, 400, errors.New("请填写扩展名称"))
		return
	}
	if v.Kind != "skill" && v.Kind != "mcp" && v.Kind != "plugin" {
		problem(w, 400, errors.New("不支持的扩展类型"))
		return
	}
	if v.Kind == "mcp" {
		if e := validateURL(v.URL); e != nil {
			problem(w, 400, e)
			return
		}
		v.Status = "已配置，尚未连接"
	} else if v.Kind == "skill" {
		if strings.TrimSpace(v.Content) == "" {
			problem(w, 400, errors.New("请填写 SKILL.md 内容"))
			return
		}
		v.Status = "可用于任务"
	} else {
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Skills  []struct {
				Name    string `json:"name"`
				Content string `json:"content"`
			} `json:"skills"`
		}
		if e := json.Unmarshal([]byte(v.Content), &manifest); e != nil || manifest.Name == "" || manifest.Version == "" {
			problem(w, 400, errors.New("插件清单需包含 name、version 和可选 skills 数组"))
			return
		}
		v.Version = manifest.Version
		v.Status = "声明式技能包"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := -1
	for i, old := range a.state.Extensions {
		if old.ID == v.ID {
			idx = i
		}
	}
	if idx < 0 {
		v.Builtin = false
		v.Category = ""
		v.SourceRoleID = ""
		v.Dependencies = nil
		v.KnowledgeIDs = nil
		v.ID = id()
		a.state.Extensions = append(a.state.Extensions, v)
	} else {
		old := a.state.Extensions[idx]
		if old.Builtin {
			if old.Name != v.Name || old.Content != v.Content || old.Description != v.Description || v.Kind != "skill" {
				problem(w, 400, errors.New("内置技能可启停；修改请先复制为自定义技能"))
				return
			}
			old.Enabled = v.Enabled
			v = old
		} else {
			v.Builtin = false
			v.Category = ""
			v.SourceRoleID = old.SourceRoleID
			if old.SourceRoleID != "" {
				v.Dependencies = old.Dependencies
				v.KnowledgeIDs = old.KnowledgeIDs
				v.Category = old.Category
			}
			if old.SourceRoleID == "" {
				v.Dependencies = nil
				v.KnowledgeIDs = nil
			}
		}
		a.state.Extensions[idx] = v
	}
	if e := a.saveLocked(); e != nil {
		problem(w, 500, e)
		return
	}
	respond(w, 200, v)
}
func (a *App) deleteExtension(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, v := range a.state.Extensions {
		if v.ID == r.PathValue("id") {
			if v.Builtin {
				problem(w, 400, errors.New("内置技能请使用停用开关；自定义副本可以删除"))
				return
			}
			a.state.Extensions = append(a.state.Extensions[:i], a.state.Extensions[i+1:]...)
			if e := a.saveLocked(); e != nil {
				problem(w, 500, e)
				return
			}
			respond(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, errors.New("扩展不存在"))
}
func (a *App) archiveTask(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	task := a.taskLocked(r.PathValue("id"))
	if task == nil {
		problem(w, 404, errors.New("会话不存在"))
		return
	}
	conversationID := task.ConversationID
	if conversationID == "" {
		conversationID = task.ID
	}
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if t.ID == conversationID || t.ConversationID == conversationID {
			t.Archived = true
		}
	}
	if e := a.saveLocked(); e != nil {
		problem(w, 500, e)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

var httpClient = &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	return fmt.Errorf("服务发生重定向，请直接配置最终地址")
}}
