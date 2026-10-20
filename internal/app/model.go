package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func generate(ctx context.Context, m Model, system, prompt string) (Deliverable, error) {
	return generateWithImages(ctx, m, system, prompt, nil)
}

type modelImage struct{ MIME, Data string }

type modelReceiptKey struct{}
type modelReceipt struct {
	Endpoint, RequestedModel, ResponseModel string
	Status                                  int
}

func generateWithImages(ctx context.Context, m Model, system, prompt string, images []modelImage) (Deliverable, error) {
	text, e := callModelText(ctx, m, system, prompt, images, true)
	if e != nil {
		return Deliverable{}, e
	}
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	var d Deliverable
	if e := json.Unmarshal([]byte(text), &d); e != nil {
		return d, errors.New("模型没有返回可解析的交付结构，请重试或更换模型；未生成虚假成果")
	}
	if d.Title == "" {
		d.Title = "办公成果"
	}
	if len(d.Sections) > 40 || len(d.Rows) > 10000 || len(d.Headers) > 100 {
		return d, errors.New("生成内容超出本版支持范围（40 章节 / 10000 行 / 100 列）")
	}
	if len(d.Sections) == 0 && len(d.Rows) == 0 {
		return d, errors.New("模型返回空内容，未创建成果")
	}
	return d, nil
}

// Conversation replies are text, not office Deliverable JSON.
func callModelText(ctx context.Context, m Model, system, prompt string, images []modelImage, structured bool) (string, error) {
	responseMIME := "text/plain"
	if structured {
		responseMIME = "application/json"
	}
	key, e := unseal(m.Secret)
	if e != nil {
		return "", e
	}
	endpoint := m.BaseURL
	var payload any
	switch m.Family {
	case "openai":
		endpoint += "/chat/completions"
		var content any = prompt
		if len(images) > 0 {
			parts := []any{map[string]string{"type": "text", "text": prompt}}
			for _, image := range images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + image.MIME + ";base64," + image.Data}})
			}
			content = parts
		}
		payload = map[string]any{"model": m.ModelID, "messages": []map[string]any{{"role": "system", "content": system}, {"role": "user", "content": content}}, "temperature": 0.3}
	case "anthropic":
		endpoint += "/messages"
		var content any = prompt
		if len(images) > 0 {
			parts := []any{}
			for _, image := range images {
				parts = append(parts, map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": image.MIME, "data": image.Data}})
			}
			content = append(parts, map[string]string{"type": "text", "text": prompt})
		}
		payload = map[string]any{"model": m.ModelID, "max_tokens": 8192, "system": system, "messages": []map[string]any{{"role": "user", "content": content}}}
	case "gemini":
		endpoint += "/models/" + url.PathEscape(m.ModelID) + ":generateContent"
		parts := []any{map[string]string{"text": prompt}}
		for _, image := range images {
			parts = append(parts, map[string]any{"inlineData": map[string]string{"mimeType": image.MIME, "data": image.Data}})
		}
		payload = map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}}, "contents": []any{map[string]any{"role": "user", "parts": parts}}, "generationConfig": map[string]any{"responseMimeType": responseMIME}}
	default:
		return "", errors.New("未知模型协议")
	}
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		switch m.Family {
		case "anthropic":
			req.Header.Set("x-api-key", key)
		case "gemini":
			req.Header.Set("x-goog-api-key", key)
		default:
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	if m.Family == "anthropic" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, e := httpClient.Do(req)
	if e != nil {
		return "", fmt.Errorf("模型请求失败：%w", e)
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return "", e
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("模型返回 HTTP %d；请检查地址、模型 ID、密钥及额度（响应正文不写入日志）", resp.StatusCode)
	}
	var body struct {
		Model        string `json:"model"`
		ModelVersion string `json:"modelVersion"`
		Choices      []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if e = json.Unmarshal(raw, &body); e != nil {
		return "", errors.New("模型响应不是有效 JSON")
	}
	if record, ok := ctx.Value(modelReceiptKey{}).(func(modelReceipt)); ok {
		responseModel := body.Model
		if responseModel == "" {
			responseModel = body.ModelVersion
		}
		// The receipt contains no headers, credentials, query strings or response body.
		if len(responseModel) > 128 || strings.ContainsAny(responseModel, "\r\n") || (key != "" && strings.Contains(responseModel, key)) {
			responseModel = "字段已隐藏"
		}
		record(modelReceipt{Endpoint: req.URL.Scheme + "://" + req.URL.Host + req.URL.EscapedPath(), RequestedModel: m.ModelID, ResponseModel: responseModel, Status: resp.StatusCode})
	}
	text := ""
	switch m.Family {
	case "openai":
		if len(body.Choices) > 0 {
			text = body.Choices[0].Message.Content
		}
	case "anthropic":
		for _, p := range body.Content {
			text += p.Text
		}
	case "gemini":
		if len(body.Candidates) > 0 {
			for _, p := range body.Candidates[0].Content.Parts {
				text += p.Text
			}
		}
	}
	text = strings.TrimSpace(text)
	// Even a misbehaving gateway echoing an authorization header must not persist
	// credentials into member reports, downstream prompts or generated files.
	if key != "" {
		text = strings.ReplaceAll(text, key, "[凭据已隐藏]")
	}
	if text == "" {
		return "", errors.New("模型未返回回复正文，请检查模型 ID、接口兼容性或重试；本次任务未完成")
	}
	return text, nil
}
