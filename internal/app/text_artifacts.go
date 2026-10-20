package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const maxTextArtifact = 512 << 10

func validateTextArtifact(kind, content string) error {
	if len(content) == 0 || len(content) > maxTextArtifact || strings.TrimSpace(content) == "" || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return errors.New("文件内容必须是非空 UTF-8 文本，最多 512 KB")
	}
	if kind == "html" {
		lower := strings.ToLower(content)
		if !strings.Contains(lower, "<html") || !strings.Contains(lower, "</html>") {
			return errors.New("网页需要完整的 HTML 文档，未保存不完整页面")
		}
		if _, err := html.Parse(strings.NewReader(content)); err != nil {
			return errors.New("HTML 内容无法解析")
		}
	} else if kind != "md" {
		return errors.New("当前文本编辑仅支持 Markdown 和 HTML")
	}
	return nil
}

func (a *App) saveTextArtifact(ctx context.Context, projectID, taskID, parentID, name, kind, content string) (Asset, error) {
	if err := validateTextArtifact(kind, content); err != nil {
		return Asset{}, err
	}
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	assetID := id()
	relative := filepath.ToSlash(filepath.Join("files", assetID+"."+kind))
	path := filepath.Join(a.root, relative)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Asset{}, err
	}
	_, err = f.WriteString(content)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		_ = os.Remove(path)
		return Asset{}, err
	}
	name = strings.TrimSuffix(safeName(name), "."+kind) + "." + kind
	validation := "UTF-8 Markdown 文件；原文件保留，新版本另存"
	if kind == "html" {
		validation = "完整 HTML 可解析；桌面预览仅支持单文件内联 CSS/JS，禁用网络和桌面桥接"
	}
	asset := Asset{ID: assetID, ProjectID: projectID, TaskID: taskID, ParentID: parentID, Name: name, Kind: kind, Size: int64(len(content)), Path: relative, CreatedAt: now(), Preview: limitText(content, 20000), Validation: validation}
	if err := a.addAsset(asset); err != nil {
		_ = os.Remove(path)
		return Asset{}, err
	}
	return asset, nil
}

func (a *App) textAsset(w http.ResponseWriter, r *http.Request) {
	asset, err := a.asset(r.PathValue("id"))
	if err != nil {
		problem(w, 404, err)
		return
	}
	if asset.Kind != "md" && asset.Kind != "html" {
		problem(w, 400, errors.New("仅支持 Markdown 和 HTML 文本"))
		return
	}
	if asset.Size > maxTextArtifact {
		problem(w, 400, errors.New("文件超过 512 KB 预览上限"))
		return
	}
	path, err := a.filePath(asset)
	if err != nil {
		problem(w, 404, err)
		return
	}
	content, err := readDocument(path, asset.Kind)
	if err != nil {
		problem(w, 400, err)
		return
	}
	if len(content) > maxTextArtifact {
		problem(w, 400, errors.New("文件超过 512 KB 预览上限"))
		return
	}
	respond(w, 200, map[string]string{"content": content})
}

func (a *App) createTextVersion(w http.ResponseWriter, r *http.Request) {
	asset, err := a.asset(r.PathValue("id"))
	if err != nil {
		problem(w, 404, err)
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &in) {
		return
	}
	next, err := a.saveTextArtifact(r.Context(), asset.ProjectID, asset.TaskID, asset.ID, asset.Name, asset.Kind, in.Content)
	if err != nil {
		problem(w, 400, err)
		return
	}
	next.Path = ""
	respond(w, 201, next)
}

func unfenceText(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if end := strings.IndexByte(content, '\n'); end >= 0 && strings.HasSuffix(content, "```") {
			content = strings.TrimSuffix(content[end+1:], "```")
		}
	}
	return strings.TrimSpace(content)
}

var htmlStart = regexp.MustCompile(`(?i)<html(?:\s|>)`)

func artifactBody(content, kind string) string {
	if kind == "html" {
		// Providers sometimes wrap valid HTML in prose and a Markdown fence.
		// Never save that prose as page content.
		loc := htmlStart.FindStringIndex(content)
		end := strings.LastIndex(strings.ToLower(content), "</html>")
		if loc != nil && end >= loc[1] {
			return "<!doctype html>\n" + content[loc[0]:end+len("</html>")]
		}
		return strings.TrimSpace(content)
	}

	if kind == "html" || strings.HasPrefix(strings.TrimSpace(content), "```markdown\n") || strings.HasPrefix(strings.TrimSpace(content), "```md\n") {
		return unfenceText(content)
	}
	return content
}
