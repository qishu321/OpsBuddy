package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrEmptyFolder = errors.New("文件夹中没有可导入的文件")

// ImportLocalFolder is reachable only through the desktop folder picker. It
// copies regular files, never follows links, and validates the whole selection first.
func (a *App) ImportLocalFolder(projectID, folder string) ([]Asset, error) {
	root, err := filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("请选择有效文件夹")
	}
	paths := []string{}
	var total int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("文件夹包含链接，请选择不含链接的材料目录")
		}
		actual, e := filepath.EvalSymlinks(path)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, actual)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return errors.New("文件夹包含指向外部的路径")
		}
		if entry.IsDir() {
			return nil
		}
		file, e := entry.Info()
		if e != nil {
			return e
		}
		if !file.Mode().IsRegular() {
			return errors.New("文件夹包含特殊文件")
		}
		paths = append(paths, path)
		total += file.Size()
		if len(paths) > 16 || total > 32<<20 {
			return errors.New("文件夹引用最多 16 个文件、合计 32 MB，请拆分材料目录")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, ErrEmptyFolder
	}
	assets, err := a.ImportLocalFiles(projectID, paths)
	if err != nil {
		return assets, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range assets {
		relative, _ := filepath.Rel(root, paths[i])
		assets[i].SourceName = filepath.ToSlash(filepath.Join(filepath.Base(root), relative))
		for j := range a.state.Assets {
			if a.state.Assets[j].ID == assets[i].ID {
				a.state.Assets[j].SourceName = assets[i].SourceName
			}
		}
	}
	if err = a.saveLocked(); err != nil {
		return nil, err
	}
	return assets, nil
}

// ImportLocalFiles accepts paths chosen through the desktop's native file dialog.
// It copies inputs into the managed workspace; it never moves the originals.
func (a *App) ImportLocalFiles(projectID string, paths []string) ([]Asset, error) {
	if len(paths) == 0 {
		return []Asset{}, nil
	}
	if len(paths) > 16 {
		return nil, errors.New("每次最多选择 16 个文件")
	}
	a.mu.RLock()
	valid := false
	for _, p := range a.state.Projects {
		if p.ID == projectID {
			valid = true
		}
	}
	a.mu.RUnlock()
	if !valid {
		return nil, errors.New("请选择有效项目")
	}
	var total int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("无法读取 %s：%w", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("请选择普通文件，不支持目录或设备")
		}
		total += info.Size()
		if total > 32<<20 {
			return nil, errors.New("单次导入总大小最多 32 MB")
		}
	}
	result := make([]Asset, 0, len(paths))
	for _, path := range paths {
		assetID := id()
		name := safeName(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(name))
		rel := filepath.ToSlash(filepath.Join("files", assetID+ext))
		destination := filepath.Join(a.root, filepath.FromSlash(rel))
		src, err := os.Open(path)
		if err != nil {
			return result, err
		}
		dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return result, err
		}
		n, err := io.Copy(dst, io.LimitReader(src, 32<<20+1))
		src.Close()
		closeErr := dst.Close()
		if err == nil {
			err = closeErr
		}
		if n > 32<<20 {
			err = errors.New("文件在导入过程中变大，已停止复制")
		}
		if err != nil {
			os.Remove(destination)
			return result, err
		}
		v := Asset{ID: assetID, Name: name, Kind: strings.TrimPrefix(ext, "."), Size: n, ProjectID: projectID, CreatedAt: now(), Path: rel, Validation: "从本机导入的副本，原文件保留"}
		if err = a.addAsset(v); err != nil {
			os.Remove(destination)
			return result, err
		}
		v.Path = ""
		result = append(result, v)
	}
	return result, nil
}

func (a *App) LocalAsset(assetID string) (Asset, string, error) {
	v, err := a.asset(assetID)
	if err != nil {
		return v, "", err
	}
	p, err := a.filePath(v)
	v.Path = ""
	return v, p, err
}

// ExportAssetTo is called only after a native save dialog returns the destination.
func (a *App) ExportAssetTo(assetID, destination string) error {
	_, srcPath, err := a.LocalAsset(assetID)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return errors.New("保存目录不存在")
	}
	root, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, parent)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return errors.New("请选择工作空间数据目录以外的位置，避免覆盖应用数据")
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	var reader io.Reader = src
	asset, err := a.asset(assetID)
	if err != nil {
		return err
	}
	if asset.Kind == "code" {
		project, e := a.loadCodeProject(asset)
		if e != nil {
			return e
		}
		raw, e := codeProjectArchive(project)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(raw)
	}
	tmp, err := os.CreateTemp(parent, ".opsbuddy-export-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = io.Copy(tmp, reader); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, abs)
}

func (a *App) ActiveTaskCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	n := 0
	for _, t := range a.state.Tasks {
		if activeTask(t.Status) {
			n++
		}
	}
	for _, r := range a.state.WorkflowRuns {
		if r.Status == "running" || r.Status == "queued" {
			n++
		}
	}
	return n
}
