package app

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type KnowledgeBase struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}
type KnowledgeChunk struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Page  int    `json:"page,omitempty"`
}
type KnowledgeDocument struct {
	ID          string           `json:"id"`
	BaseID      string           `json:"baseId"`
	AssetID     string           `json:"assetId"`
	Name        string           `json:"name"`
	Status      string           `json:"status"`
	Error       string           `json:"error,omitempty"`
	ChunkCount  int              `json:"chunkCount"`
	ContentHash string           `json:"contentHash"`
	IndexedAt   string           `json:"indexedAt"`
	Chunks      []KnowledgeChunk `json:"chunks,omitempty"`
}
type KnowledgeHit struct {
	BaseID      string  `json:"baseId"`
	DocumentID  string  `json:"documentId"`
	AssetID     string  `json:"assetId"`
	Name        string  `json:"name"`
	ChunkIndex  int     `json:"chunkIndex"`
	Page        int     `json:"page,omitempty"`
	Text        string  `json:"text"`
	Score       float64 `json:"score"`
	ContentHash string  `json:"contentHash"`
}

func chunkText(text string) []KnowledgeChunk {
	chunks := []KnowledgeChunk{}
	page := 0
	rx := regexp.MustCompile(`\[第\s*(\d+)\s*页\]`)
	parts := []struct {
		text string
		page int
	}{}
	var lines strings.Builder
	flush := func() {
		if lines.Len() > 0 {
			parts = append(parts, struct {
				text string
				page int
			}{lines.String(), page})
			lines.Reset()
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if match := rx.FindStringSubmatch(line); len(match) > 1 {
			flush()
			page, _ = strconv.Atoi(match[1])
		}
		lines.WriteString(line + "\n")
	}
	flush()
	for _, part := range parts {
		r := []rune(strings.TrimSpace(part.text))
		for start := 0; start < len(r); start += 500 {
			end := start + 600
			if end > len(r) {
				end = len(r)
			}
			chunks = append(chunks, KnowledgeChunk{Index: len(chunks), Text: string(r[start:end]), Page: part.page})
			if end == len(r) {
				break
			}
		}
	}
	return chunks
}
func searchTokens(text string) []string {
	seen := map[string]bool{}
	tokens := []string{}
	add := func(t string) {
		if len([]rune(t)) >= 2 && !seen[t] {
			seen[t] = true
			tokens = append(tokens, t)
		}
	}
	r := []rune(strings.ToLower(text))
	for i := 0; i < len(r); {
		if unicode.Is(unicode.Han, r[i]) {
			j := i
			for j < len(r) && unicode.Is(unicode.Han, r[j]) {
				j++
			}
			for k := i; k+1 < j; k++ {
				add(string(r[k : k+2]))
			}
			i = j
		} else if unicode.IsLetter(r[i]) || unicode.IsDigit(r[i]) {
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j])) && !unicode.Is(unicode.Han, r[j]) {
				j++
			}
			add(string(r[i:j]))
			i = j
		} else {
			i++
		}
	}
	return tokens
}
func (a *App) searchKnowledgeLocked(projectID string, baseIDs []string, query string, limit int) ([]KnowledgeHit, error) {
	selected := map[string]bool{}
	for _, baseID := range baseIDs {
		found := false
		for _, b := range a.state.KnowledgeBases {
			if b.ID == baseID && b.ProjectID == projectID {
				found = true
				selected[baseID] = true
			}
		}
		if !found {
			return nil, errors.New("知识库不存在或不属于当前项目")
		}
	}
	if len(query) > 30000 {
		return nil, errors.New("检索内容过长")
	}
	if limit < 1 || limit > 20 {
		limit = 6
	}
	tokens := searchTokens(query)
	hits := []KnowledgeHit{}
	docs := []KnowledgeDocument{}
	for _, d := range a.state.KnowledgeDocuments {
		if selected[d.BaseID] && d.Status == "ready" {
			docs = append(docs, d)
		}
	}
	frequencies := map[string]int{}
	for _, d := range docs {
		for _, token := range tokens {
			for _, ch := range d.Chunks {
				if strings.Contains(strings.ToLower(ch.Text), token) {
					frequencies[token]++
					break
				}
			}
		}
	}
	for _, d := range docs {
		for _, ch := range d.Chunks {
			lower := strings.ToLower(ch.Text)
			score := 0.0
			for _, token := range tokens {
				count := strings.Count(lower, token)
				if count > 0 {
					weight := math.Log(1 + float64(len(docs)+1)/float64(frequencies[token]+1))
					score += weight * (1 + math.Log(float64(count)))
				}
			}
			if strings.TrimSpace(query) != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(query))) {
				score += 5
			}
			if score > 0 {
				hits = append(hits, KnowledgeHit{BaseID: d.BaseID, DocumentID: d.ID, AssetID: d.AssetID, Name: d.Name, ChunkIndex: ch.Index, Page: ch.Page, Text: ch.Text, Score: score, ContentHash: d.ContentHash})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}
func (a *App) createKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	var b KnowledgeBase
	if !decode(w, r, &b) {
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len(b.Name) > 160 {
		problem(w, 400, errors.New("请填写知识库名称（最多 160 字节）"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for _, p := range a.state.Projects {
		if p.ID == b.ProjectID {
			found = true
		}
	}
	if !found {
		problem(w, 400, errors.New("项目不存在"))
		return
	}
	b.ID = id()
	b.CreatedAt = now()
	a.state.KnowledgeBases = append(a.state.KnowledgeBases, b)
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 201, b)
}
func (a *App) indexKnowledgeAssets(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AssetIDs []string `json:"assetIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.AssetIDs) == 0 || len(in.AssetIDs) > 16 {
		problem(w, 400, errors.New("请选择 1–16 份资料"))
		return
	}
	a.mu.RLock()
	var base KnowledgeBase
	for _, b := range a.state.KnowledgeBases {
		if b.ID == r.PathValue("id") {
			base = b
		}
	}
	a.mu.RUnlock()
	if base.ID == "" {
		problem(w, 404, errors.New("知识库不存在"))
		return
	}
	assets := []Asset{}
	for _, assetID := range in.AssetIDs {
		v, err := a.asset(assetID)
		if err != nil || v.ProjectID != base.ProjectID {
			problem(w, 400, errors.New("资料不属于知识库项目"))
			return
		}
		assets = append(assets, v)
	}
	out := []KnowledgeDocument{}
	for _, v := range assets {
		if r.Context().Err() != nil {
			return
		}
		d := KnowledgeDocument{ID: id(), BaseID: base.ID, AssetID: v.ID, Name: v.Name, Status: "ready", IndexedAt: now()}
		path, err := a.filePath(v)
		var text string
		if err == nil {
			text, err = readDocument(path, v.Kind)
		}
		if err == nil && len([]rune(text)) > 1000000 {
			err = errors.New("文档超过 100 万字符，请拆分后导入")
		}
		if err == nil && (strings.TrimSpace(text) == "" || (v.Kind == "pdf" && strings.Contains(text, "[未检测到文本层"))) {
			err = errors.New("文档含无法提取的文本层或扫描页，需 OCR 后再导入")
		}
		if err != nil {
			d.Status = "failed"
			d.Error = err.Error()
		} else {
			d.Chunks = chunkText(text)
			d.ChunkCount = len(d.Chunks)
			d.ContentHash = contentHash(text)
		}
		a.mu.Lock()
		index := -1
		for i, old := range a.state.KnowledgeDocuments {
			if old.BaseID == base.ID && old.AssetID == v.ID {
				index = i
				d.ID = old.ID
			}
		}
		if index < 0 {
			a.state.KnowledgeDocuments = append(a.state.KnowledgeDocuments, d)
		} else {
			a.state.KnowledgeDocuments[index] = d
		}
		err = a.saveLocked()
		a.mu.Unlock()
		if err != nil {
			problem(w, 500, err)
			return
		}
		d.Chunks = nil
		out = append(out, d)
	}
	respond(w, 200, out)
}
func (a *App) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProjectID string   `json:"projectId"`
		BaseIDs   []string `json:"baseIds"`
		Query     string   `json:"query"`
		Limit     int      `json:"limit"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.mu.RLock()
	hits, err := a.searchKnowledgeLocked(in.ProjectID, in.BaseIDs, in.Query, in.Limit)
	a.mu.RUnlock()
	if err != nil {
		problem(w, 400, err)
		return
	}
	respond(w, 200, hits)
}
func (a *App) knowledgeDocument(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, d := range a.state.KnowledgeDocuments {
		if d.ID == r.PathValue("id") {
			if len(d.Chunks) > 20 {
				d.Chunks = d.Chunks[:20]
			}
			respond(w, 200, d)
			return
		}
	}
	problem(w, 404, errors.New("资料不存在"))
}
func (a *App) removeKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, d := range a.state.KnowledgeDocuments {
		if d.ID == r.PathValue("id") {
			a.state.KnowledgeDocuments = append(a.state.KnowledgeDocuments[:i], a.state.KnowledgeDocuments[i+1:]...)
			if err := a.saveLocked(); err != nil {
				problem(w, 500, err)
				return
			}
			respond(w, 200, map[string]string{"message": "已移出索引，原文件和任务引用保留"})
			return
		}
	}
	problem(w, 404, errors.New("资料不存在"))
}
func knowledgeContext(hits []KnowledgeHit) string {
	if len(hits) == 0 {
		return "\n[知识库检索结果：未命中相关片段。]\n"
	}
	var out strings.Builder
	for _, hit := range hits {
		page := ""
		if hit.Page > 0 {
			page = fmt.Sprintf(" · PDF 第 %d 页", hit.Page)
		}
		fmt.Fprintf(&out, "\n[知识库来源：%s · 片段 %d%s]\n%s\n", hit.Name, hit.ChunkIndex+1, page, hit.Text)
	}
	return out.String()
}
