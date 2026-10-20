// Package agentfixture provides deterministic, local-only model responses for
// explicit tests. It is not imported by the production application executor.
package agentfixture

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const Frontend = `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>协作计算器</title></head><body><h1>前端与 Go 协作验收</h1><label>A <input id="a" value="7"></label><label>B <input id="b" value="5"></label><button id="add">调用 Go 接口计算</button><output id="result">等待计算</output><script>
document.querySelector('#add').onclick=async()=>{try{const r=await fetch('./api/add?a='+encodeURIComponent(document.querySelector('#a').value)+'&b='+encodeURIComponent(document.querySelector('#b').value));const data=await r.json();document.querySelector('#result').textContent=String(data.sum);parent.postMessage({opsbuddyAgentCalculation:data.sum,source:data.source,bridge:!!window.go},'*')}catch(e){parent.postMessage({opsbuddyAgentError:e.message},'*')}};
addEventListener('message',e=>{if(e.source===parent&&e.data?.opsbuddyAgentClick)document.querySelector('#add').click()});parent.postMessage({opsbuddyAgentReady:true},'*');
</script></body></html>`

const Backend = `package main
import("encoding/json";"log";"net/http";"os";"strconv")
func main(){mux:=http.NewServeMux();mux.HandleFunc("/api/add",func(w http.ResponseWriter,r *http.Request){a,e1:=strconv.Atoi(r.URL.Query().Get("a"));b,e2:=strconv.Atoi(r.URL.Query().Get("b"));if e1!=nil||e2!=nil{http.Error(w,"invalid integer",400);return};w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(map[string]any{"sum":a+b,"source":"go-backend"})});mux.Handle("/",http.FileServer(http.Dir(".")));log.Println("agent-go-backend-ready");log.Fatal(http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"),mux))}`

type Receipt struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Stage    string `json:"stage"`
	At       string `json:"at"`
}
type Server struct {
	mu            sync.Mutex
	receipts      []Receipt
	recoveryCalls int
}

func (s *Server) Receipts() []Receipt {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Receipt{}, s.receipts...)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/evidence" {
		_ = json.NewEncoder(w).Encode(s.Receipts())
		return
	}
	var in struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Messages) < 2 {
		http.Error(w, "fixture expects chat request", 400)
		return
	}
	system, prompt := in.Messages[0].Content, in.Messages[1].Content
	if strings.Contains(prompt, "CANCEL_COLLAB") {
		<-r.Context().Done()
		return
	}
	if strings.Contains(prompt, "RECOVER_COLLAB") {
		s.mu.Lock()
		s.recoveryCalls++
		first := s.recoveryCalls == 1
		s.mu.Unlock()
		if first {
			<-r.Context().Done()
			return
		}
	}
	stage, reply := "final", "协作验收内容：材料中明确预算 120 元，数量 3，未增加未知事实。"
	if strings.Contains(system, "技能选择器") {
		stage = "skills"
		reply = `{"sections":[{"body":"本次无需额外技能"}],"rows":[]}`
	} else if strings.Contains(system, "你是本项目任务的主协调者") {
		stage = "plan"
		var catalog struct {
			Models []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"models"`
		}
		if i := strings.Index(prompt, "本次可用配置："); i >= 0 {
			_ = json.NewDecoder(strings.NewReader(prompt[i+len("本次可用配置："):])).Decode(&catalog)
		}
		front, back := "", ""
		for _, m := range catalog.Models {
			if strings.Contains(m.Name, "front") {
				front = m.ID
			}
			if strings.Contains(m.Name, "back") {
				back = m.ID
			}
		}
		if strings.Contains(prompt, "CODE_COLLAB") {
			reply = encode(map[string]any{"summary": "前端与 Go 后端并行交付，独立审阅，主任务集成启动", "members": []any{
				map[string]any{"id": "front", "role": "worker", "name": "前端", "modelId": front, "prompt": "FRONTEND_FIXTURE：实现可点击页面，调用 ./api/add", "kind": "code", "ownedPaths": []string{"index.html"}, "dependsOn": []string{}, "acceptance": "按钮请求真实 Go 接口"},
				map[string]any{"id": "back", "role": "worker", "name": "Go后端", "modelId": back, "prompt": "BACKEND_FIXTURE：实现整数加法 API", "kind": "code", "ownedPaths": []string{"main.go", "go.mod"}, "dependsOn": []string{}, "acceptance": "非法参数返回400，正确参数计算真实结果"},
				map[string]any{"id": "review", "role": "reviewer", "name": "独立审阅", "prompt": "核对前后端接口契约", "kind": "chat", "dependsOn": []string{"front", "back"}, "acceptance": "路径、参数和启动约定一致"},
			}})
		} else {
			reply = encode(map[string]any{"summary": "资料分析→撰写→独立审阅→统一办公文件", "members": []any{
				map[string]any{"id": "analysis", "role": "explorer", "name": "资料分析", "prompt": "OFFICE_ANALYSIS：提取预算与数量", "kind": "chat", "dependsOn": []string{}, "acceptance": "数字与输入一致"},
				map[string]any{"id": "draft", "role": "worker", "name": "办公撰写", "modelId": back, "prompt": "OFFICE_DRAFT：根据分析撰写报告", "kind": "docx", "dependsOn": []string{"analysis"}, "acceptance": "预算120，数量3"},
				map[string]any{"id": "review", "role": "reviewer", "name": "独立审阅", "prompt": "核对数字与原始材料", "kind": "chat", "dependsOn": []string{"draft"}, "acceptance": "原材料一致，未捏造"},
			}})
		}
	} else if strings.Contains(system, "你是独立审阅者") {
		stage = "review"
		reply = `{"status":"pass","summary":"提供的交付与接口/数字约束一致；这里只审阅源码和内容，实际启动与按钮测试由后续本地验收执行。","issues":[]}`
	} else if strings.Contains(system, "本节点负责路径") {
		stage = "candidate"
		files := []map[string]string{}
		if strings.Contains(prompt, "当前职责：FRONTEND_FIXTURE") {
			files = append(files, map[string]string{"path": "index.html", "content": Frontend})
		} else {
			files = append(files, map[string]string{"path": "main.go", "content": Backend}, map[string]string{"path": "go.mod", "content": "module agentdelivery\n\ngo 1.22\n"})
		}
		reply = encode(map[string]any{"title": "候选源码", "runtime": "go", "mode": "fragment", "entry": "", "files": files})
	} else if strings.Contains(system, "生成可运行的轻量代码项目") {
		stage = "integrate"
		reply = `{"title":"协作计算器实际交付","runtime":"go","mode":"server","entry":"main.go","files":[]}`
	} else if strings.Contains(system, "你是 OpsBuddy 办公助手") {
		stage = "office"
		reply = `{"title":"协作办公验收报告","summary":"根据上游实际资料编写","sections":[{"heading":"预算与数量","body":"预算为120元，数量为3。资料分析与审阅记录均已提供。未验证事项不冒充完成。"}]}`
	} else if strings.Contains(prompt, "当前职责：") {
		stage = "analysis"
	}
	s.mu.Lock()
	s.receipts = append(s.receipts, Receipt{r.URL.Path, in.Model, stage, time.Now().UTC().Format(time.RFC3339Nano)})
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"model": in.Model + "-fixture-actual", "choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
}
func encode(value any) string { raw, _ := json.Marshal(value); return string(raw) }
