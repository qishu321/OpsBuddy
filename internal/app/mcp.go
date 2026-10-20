package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// This first integration discovers remote tools only; execution is deliberately not exposed.
func rpc(ctx context.Context, endpoint, session string, payload any) (map[string]any, string, error) {
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if e != nil {
		return nil, "", e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
		req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	}
	resp, e := httpClient.Do(req)
	if e != nil {
		return nil, "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("MCP 返回 HTTP %d，本版仅支持无需认证的远程工具发现", resp.StatusCode)
	}
	if resp.StatusCode == 202 {
		return map[string]any{}, resp.Header.Get("Mcp-Session-Id"), nil
	}
	var result map[string]any
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		var data []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
			if line == "" && len(data) > 0 {
				var event map[string]any
				if json.Unmarshal([]byte(strings.Join(data, "\n")), &event) == nil && (event["result"] != nil || event["error"] != nil) {
					result = event
					break
				}
				data = nil
			}
		}
		e = scanner.Err()
	} else {
		e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
	}
	if e != nil || result == nil {
		return nil, "", errors.New("MCP 响应无法解析")
	}
	if result["error"] != nil {
		return nil, "", errors.New("MCP 返回协议错误，请核对服务地址与版本")
	}
	return result, resp.Header.Get("Mcp-Session-Id"), nil
}
func (a *App) testExtension(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	var x Extension
	for _, v := range a.state.Extensions {
		if v.ID == r.PathValue("id") {
			x = v
		}
	}
	a.mu.RUnlock()
	if x.Kind != "mcp" {
		problem(w, 400, errors.New("请选择 MCP 连接"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	initialized, session, e := rpc(ctx, x.URL, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "OpsBuddy", "version": "0.1.0"}}})
	if e != nil {
		problem(w, 400, e)
		return
	}
	initResult, ok := initialized["result"].(map[string]any)
	if !ok || initResult["protocolVersion"] != "2025-03-26" {
		problem(w, 400, errors.New("服务未协商本版支持的 MCP 2025-03-26 协议"))
		return
	}
	_, _, e = rpc(ctx, x.URL, session, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if e != nil {
		problem(w, 400, e)
		return
	}
	result, _, e := rpc(ctx, x.URL, session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	if e != nil {
		problem(w, 400, e)
		return
	}
	res, ok := result["result"].(map[string]any)
	if !ok {
		problem(w, 400, errors.New("工具列表响应缺失 result"))
		return
	}
	toolList, ok := res["tools"].([]any)
	if !ok {
		problem(w, 400, errors.New("工具列表响应缺失 tools"))
		return
	}
	count := len(toolList)
	status := fmt.Sprintf("发现 %d 个工具 · 尚未接入执行", count)
	a.mu.Lock()
	for i := range a.state.Extensions {
		if a.state.Extensions[i].ID == x.ID {
			a.state.Extensions[i].Status = status
		}
	}
	_ = a.saveLocked()
	a.mu.Unlock()
	respond(w, 200, map[string]string{"message": status})
}
