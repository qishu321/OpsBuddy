package main

import (
    "encoding/json"
    "errors"
    "io"
    "log"
    "mime"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"
    "unicode/utf8"
)

const (
    defaultPort   = "18086"
    dataFilePath  = "data/todos.json"
    indexFilePath = "index.html"
    maxBodyBytes  = 16 << 10
    maxTitleRunes = 120
)

type Todo struct {
    ID    int    `json:"id"`
    Title string `json:"title"`
    Done  bool   `json:"done"`
}

type Store struct {
    mu     sync.RWMutex
    path   string
    items  []Todo
    nextID int
}

var errTodoNotFound = errors.New("todo not found")

func NewStore(path string) (*Store, error) {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return nil, err
    }

    data, err := os.ReadFile(path)
    if errors.Is(err, os.ErrNotExist) {
        store := &Store{
            path:   path,
            items:  make([]Todo, 0),
            nextID: 1,
        }
        if err := writeTodosFile(path, store.items); err != nil {
            return nil, err
        }
        return store, nil
    }
    if err != nil {
        return nil, err
    }

    var items []Todo
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, err
    }
    if items == nil {
        items = make([]Todo, 0)
    }

    maxID := 0
    for _, item := range items {
        if item.ID > maxID {
            maxID = item.ID
        }
    }

    return &Store{
        path:   path,
        items:  items,
        nextID: maxID + 1,
    }, nil
}

func writeTodosFile(path string, items []Todo) error {
    data, err := json.MarshalIndent(items, "", "  ")
    if err != nil {
        return err
    }
    data = append(data, '\n')

    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".todos-*.tmp")
    if err != nil {
        return err
    }
    tmpName := tmp.Name()
    cleanup := func() {
        _ = os.Remove(tmpName)
    }

    if _, err := tmp.Write(data); err != nil {
        _ = tmp.Close()
        cleanup()
        return err
    }
    if err := tmp.Sync(); err != nil {
        _ = tmp.Close()
        cleanup()
        return err
    }
    if err := tmp.Close(); err != nil {
        cleanup()
        return err
    }
    if err := os.Rename(tmpName, path); err != nil {
        cleanup()
        return err
    }
    return nil
}

func (s *Store) List() []Todo {
    s.mu.RLock()
    defer s.mu.RUnlock()

    out := make([]Todo, len(s.items))
    copy(out, s.items)
    return out
}

func (s *Store) Add(title string) (Todo, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    oldItems := s.items
    oldNextID := s.nextID
    item := Todo{
        ID:    s.nextID,
        Title: title,
        Done:  false,
    }

    s.items = append(s.items, item)
    s.nextID++

    if err := writeTodosFile(s.path, s.items); err != nil {
        s.items = oldItems
        s.nextID = oldNextID
        return Todo{}, err
    }
    return item, nil
}

func (s *Store) SetDone(id int, done bool) (Todo, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    for i := range s.items {
        if s.items[i].ID != id {
            continue
        }

        oldItem := s.items[i]
        s.items[i].Done = done
        if err := writeTodosFile(s.path, s.items); err != nil {
            s.items[i] = oldItem
            return Todo{}, err
        }
        return s.items[i], nil
    }
    return Todo{}, errTodoNotFound
}

type server struct {
    store     *Store
    indexPath string
}

type itemsResponse struct {
    Items []Todo `json:"items"`
}

type itemResponse struct {
    Item Todo `json:"item"`
}

type errorResponse struct {
    Error string `json:"error"`
}

func (s *server) routes() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/health", s.handleHealth)
    mux.HandleFunc("/api/todos", s.handleTodos)
    mux.HandleFunc("/api/todos/", s.handleTodoByID)
    mux.HandleFunc("/", s.handleIndex)
    return mux
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        methodNotAllowed(w, http.MethodGet)
        return
    }
    writeJSON(w, http.StatusOK, struct {
        OK bool `json:"ok"`
    }{OK: true})
}

func (s *server) handleTodos(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        writeJSON(w, http.StatusOK, itemsResponse{Items: s.store.List()})
    case http.MethodPost:
        s.handleCreateTodo(w, r)
    default:
        methodNotAllowed(w, "GET, POST")
    }
}

func (s *server) handleCreateTodo(w http.ResponseWriter, r *http.Request) {
    if !hasJSONContentType(r) {
        writeError(w, http.StatusUnsupportedMediaType, "Content-Type 必须为 application/json")
        return
    }

    var req struct {
        Title string `json:"title"`
    }
    if !decodeJSONBody(w, r, &req) {
        return
    }

    title := strings.TrimSpace(req.Title)
    if title == "" {
        writeError(w, http.StatusBadRequest, "标题不能为空")
        return
    }
    if utf8.RuneCountInString(title) > maxTitleRunes {
        writeError(w, http.StatusBadRequest, "标题不能超过120个Unicode字符")
        return
    }

    item, err := s.store.Add(title)
    if err != nil {
        log.Printf("保存新增待办失败: %v", err)
        writeError(w, http.StatusInternalServerError, "保存待办数据失败，请稍后重试")
        return
    }
    writeJSON(w, http.StatusCreated, itemResponse{Item: item})
}

func (s *server) handleTodoByID(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPatch {
        methodNotAllowed(w, http.MethodPatch)
        return
    }

    rawID := strings.TrimPrefix(r.URL.Path, "/api/todos/")
    if rawID == "" || strings.Contains(rawID, "/") {
        writeError(w, http.StatusNotFound, "待办不存在")
        return
    }
    id, err := strconv.Atoi(rawID)
    if err != nil || id <= 0 {
        writeError(w, http.StatusNotFound, "待办不存在")
        return
    }

    if !hasJSONContentType(r) {
        writeError(w, http.StatusUnsupportedMediaType, "Content-Type 必须为 application/json")
        return
    }

    var req struct {
        Done *bool `json:"done"`
    }
    if !decodeJSONBody(w, r, &req) {
        return
    }
    if req.Done == nil {
        writeError(w, http.StatusBadRequest, "done 字段必须是布尔值")
        return
    }

    item, err := s.store.SetDone(id, *req.Done)
    if errors.Is(err, errTodoNotFound) {
        writeError(w, http.StatusNotFound, "待办不存在")
        return
    }
    if err != nil {
        log.Printf("保存待办状态失败: %v", err)
        writeError(w, http.StatusInternalServerError, "保存待办数据失败，请稍后重试")
        return
    }
    writeJSON(w, http.StatusOK, itemResponse{Item: item})
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        writeError(w, http.StatusNotFound, "页面不存在")
        return
    }
    if r.Method != http.MethodGet {
        methodNotAllowed(w, http.MethodGet)
        return
    }

    data, err := os.ReadFile(s.indexPath)
    if err != nil {
        log.Printf("读取 index.html 失败: %v", err)
        writeError(w, http.StatusInternalServerError, "读取首页失败")
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Header().Set("Cache-Control", "no-store")
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write(data)
}

func hasJSONContentType(r *http.Request) bool {
    raw := strings.TrimSpace(r.Header.Get("Content-Type"))
    if raw == "" {
        return true
    }
    mediaType, _, err := mime.ParseMediaType(raw)
    if err != nil {
        return false
    }
    return strings.EqualFold(mediaType, "application/json")
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
    r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields()

    if err := decoder.Decode(dst); err != nil {
        if isBodyTooLarge(err) {
            writeError(w, http.StatusRequestEntityTooLarge, "请求体过大")
        } else {
            writeError(w, http.StatusBadRequest, "请求体必须是合法的 JSON 对象")
        }
        return false
    }

    var extra any
    if err := decoder.Decode(&extra); err != io.EOF {
        if err == nil {
            writeError(w, http.StatusBadRequest, "请求体只能包含一个 JSON 对象")
        } else if isBodyTooLarge(err) {
            writeError(w, http.StatusRequestEntityTooLarge, "请求体过大")
        } else {
            writeError(w, http.StatusBadRequest, "请求体包含无效的 JSON 内容")
        }
        return false
    }
    return true
}

func isBodyTooLarge(err error) bool {
    var maxBytesError *http.MaxBytesError
    return errors.As(err, &maxBytesError)
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
    w.Header().Set("Allow", allow)
    writeError(w, http.StatusMethodNotAllowed, "请求方法不允许")
}

func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Header().Set("Cache-Control", "no-store")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(payload); err != nil {
        log.Printf("写入 JSON 响应失败: %v", err)
    }
}

func main() {
    port := strings.TrimSpace(os.Getenv("PORT"))
    if port == "" {
        port = defaultPort
    }
    portNumber, err := strconv.Atoi(port)
    if err != nil || portNumber < 1 || portNumber > 65535 {
        log.Fatalf("无效的 PORT 环境变量: %q", port)
    }

    store, err := NewStore(dataFilePath)
    if err != nil {
        log.Fatalf("初始化待办数据失败: %v", err)
    }

    app := &server{store: store, indexPath: indexFilePath}
    addr := net.JoinHostPort("127.0.0.1", port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        log.Fatalf("监听 %s 失败: %v", addr, err)
    }

    httpServer := &http.Server{
        Handler:           app.routes(),
        ReadHeaderTimeout: 5 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    log.Printf("中文办公待办看板服务正在运行: http://%s", listener.Addr().String())
    if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Fatalf("HTTP 服务异常退出: %v", err)
    }
}
