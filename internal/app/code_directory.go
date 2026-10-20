package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Materialize a legacy archive only when explicitly opening its source directory.
// The original archive stays on disk; future reads use the editable source files.
func (a *App) CodeSourceDirectory(assetID string) (string, error) {
	a.extractMu.Lock()
	defer a.extractMu.Unlock()
	asset, err := a.asset(assetID)
	if err != nil {
		return "", err
	}
	project, err := a.loadCodeProject(asset)
	if err != nil {
		return "", err
	}
	full, err := a.filePath(asset)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(full, ".json") {
		return filepath.Dir(full), nil
	}
	folder, err := os.MkdirTemp(filepath.Join(a.root, "files"), asset.ID+"-source-")
	if err != nil {
		return "", err
	}
	for _, file := range append(append([]CodeFile{}, project.Files...), bundledProjectFiles(project)...) {
		target := filepath.Join(folder, filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err = os.WriteFile(target, []byte(file.Content), 0600); err != nil {
			return "", err
		}
	}
	raw, _ := json.Marshal(project)
	if err = os.WriteFile(filepath.Join(folder, "opsbuddy-project.json"), raw, 0600); err != nil {
		return "", err
	}
	manifest, err := filepath.Rel(a.root, filepath.Join(folder, "opsbuddy-project.json"))
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Assets {
		if a.state.Assets[i].ID == assetID {
			a.state.Assets[i].Path = filepath.ToSlash(manifest)
			a.state.Assets[i].Name = strings.TrimSuffix(asset.Name, ".zip")
			break
		}
	}
	if err = a.saveLocked(); err != nil {
		return "", err
	}
	return folder, nil
}
