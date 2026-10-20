package app

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// Vue 3.5.44, official production builds; MIT license accompanies exports.
// These assets are loaded only inside a user's generated project, never the desktop UI.
//
//go:embed runtime_assets/*
var codeRuntimeAssets embed.FS

const runtimeAssetDir = "__opsbuddy_runtime__"

// Reject the specific incomplete mount shape that leaves a generated Vue page
// permanently displaying its loading placeholder. Existing versions remain readable.
func validateCodeUI(p CodeProject) error {
	var all strings.Builder
	for _, file := range p.Files {
		all.WriteString(file.Content)
		all.WriteByte('\n')
	}
	source := all.String()
	if !strings.Contains(source, "createApp") || regexp.MustCompile(`(?i)\b(template\s*:|render\s*[:(])`).MatchString(source) {
		return nil
	}
	for _, file := range p.Files {
		if !strings.EqualFold(path.Ext(file.Path), ".html") {
			continue
		}
		matches := regexp.MustCompile(`(?is)<div\s+id=["']([^"']+)["']\s*>\s*<p\s+class=["']loading["']\s*>[^<]*</p>\s*</div>`).FindAllStringSubmatch(file.Content, -1)
		for _, match := range matches {
			mount := regexp.MustCompile(`\.mount\(\s*["']#` + regexp.QuoteMeta(match[1]) + `["']\s*\)`)
			if mount.MatchString(source) {
				return errors.New("Vue 页面只有加载占位，缺少实际界面模板；请补齐挂载容器内容、template 或 render")
			}
		}
	}
	return nil
}

var vueCDN = regexp.MustCompile(`(?i)(https?:)?//(?:unpkg\.com/vue(?:@[^/\s"'<>` + "`" + `]+)?/dist/|cdn\.jsdelivr\.net/npm/vue(?:@[^/\s"'<>` + "`" + `]+)?/dist/|cdnjs\.cloudflare\.com/ajax/libs/vue/[^/]+/)(vue(?:\.runtime)?\.(?:global|esm-browser)(?:\.prod)?\.js)(?:\?[^\s"'<>` + "`" + `]*)?`)

func localVueReferences(content string) string {
	return vueCDN.ReplaceAllStringFunc(content, func(raw string) string {
		name := "vue.global.prod.js"
		if strings.Contains(strings.ToLower(raw), "esm-browser") {
			name = "vue.esm-browser.prod.js"
		}
		return "./" + runtimeAssetDir + "/" + name
	})
}
func normalizeCodeProject(p *CodeProject) {
	for i := range p.Files {
		p.Files[i].Content = localVueReferences(p.Files[i].Content)
		ext := path.Ext(p.Files[i].Path)
		if ext == ".html" || ext == ".js" || ext == ".mjs" {
			depth := strings.Count(p.Files[i].Path, "/")
			if depth > 0 {
				p.Files[i].Content = strings.ReplaceAll(p.Files[i].Content, "\"./"+runtimeAssetDir+"/", "\""+strings.Repeat("../", depth)+runtimeAssetDir+"/")
				p.Files[i].Content = strings.ReplaceAll(p.Files[i].Content, "'./"+runtimeAssetDir+"/", "'"+strings.Repeat("../", depth)+runtimeAssetDir+"/")
			}
		}
	}
}
func bundledProjectFiles(p CodeProject) []CodeFile {
	var files []CodeFile
	for _, name := range []string{"vue.global.prod.js", "vue.esm-browser.prod.js"} {
		used := false
		for _, f := range p.Files {
			if strings.Contains(f.Content, runtimeAssetDir+"/"+name) {
				used = true
				break
			}
		}
		if used {
			raw, _ := codeRuntimeAssets.ReadFile("runtime_assets/" + name)
			files = append(files, CodeFile{Path: runtimeAssetDir + "/" + name, Content: string(raw)})
		}
	}
	if len(files) > 0 {
		raw, _ := codeRuntimeAssets.ReadFile("runtime_assets/VUE-LICENSE.txt")
		files = append(files, CodeFile{Path: runtimeAssetDir + "/VUE-LICENSE.txt", Content: string(raw)})
	}
	return files
}

func serveBundledRuntime(w http.ResponseWriter, r *http.Request, suffix string) bool {
	if !strings.HasPrefix(suffix, runtimeAssetDir+"/") {
		return false
	}
	name := strings.TrimPrefix(suffix, runtimeAssetDir+"/")
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return true
	}
	if name != "vue.global.prod.js" && name != "vue.esm-browser.prod.js" && name != "VUE-LICENSE.txt" {
		http.NotFound(w, r)
		return true
	}
	assets, _ := fs.Sub(codeRuntimeAssets, "runtime_assets")
	clone := r.Clone(r.Context())
	clone.URL.Path = "/" + name
	http.FileServer(http.FS(assets)).ServeHTTP(w, clone)
	return true
}

// Runs only in the isolated preview origin. The parent validates event.source.
const previewHealthScript = `<script>(function(){function send(kind,message){parent.postMessage({type:'opsbuddy-preview-health',kind:kind,message:String(message||'').slice(0,600)},'*')}addEventListener('error',function(e){send('error',e.message||(e.target&&e.target.src?'资源加载失败：'+e.target.src:'页面资源加载失败'))},true);addEventListener('unhandledrejection',function(e){send('error',e.reason&&e.reason.message||e.reason)});addEventListener('securitypolicyviolation',function(e){send('error','预览阻止了外部资源：'+e.blockedURI)});addEventListener('load',function(){send('ready','')})})();</script>`

func previewHTML(body string) string {
	body = localVueReferences(body)
	if index := strings.Index(strings.ToLower(body), "<head>"); index >= 0 {
		return body[:index+6] + previewHealthScript + body[index+6:]
	}
	if index := strings.Index(strings.ToLower(body), "<html"); index >= 0 {
		if end := strings.Index(body[index:], ">"); end >= 0 {
			index += end + 1
			return body[:index] + previewHealthScript + body[index:]
		}
	}
	return previewHealthScript + body
}
