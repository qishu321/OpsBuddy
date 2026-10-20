package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeImportAndExport(t *testing.T) {
	a := newTestApp(t)
	inputDir := t.TempDir()
	source := filepath.Join(inputDir, "中文材料.txt")
	if err := os.WriteFile(source, []byte("原始材料，保留中文"), 0600); err != nil {
		t.Fatal(err)
	}
	assets, err := a.ImportLocalFiles("inbox", []string{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Path != "" {
		t.Fatal("invalid public asset", assets)
	}
	_, managed, err := a.LocalAsset(assets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if managed == source {
		t.Fatal("source was not copied")
	}
	output := filepath.Join(t.TempDir(), "另存.txt")
	if err = a.ExportAssetTo(assets[0].ID, output); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(output)
	if string(got) != "原始材料，保留中文" {
		t.Fatal(string(got))
	}
	if err = a.ExportAssetTo(assets[0].ID, filepath.Join(a.root, "state.db")); err == nil {
		t.Fatal("internal database overwrite was allowed")
	}
	before, _ := os.ReadFile(source)
	if string(before) != string(got) {
		t.Fatal("source changed")
	}
	if _, err = a.ImportLocalFiles("unknown", []string{source}); err == nil {
		t.Fatal("unknown project accepted")
	}
	if _, err = a.ImportLocalFiles("inbox", []string{inputDir}); err == nil {
		t.Fatal("directory accepted as file")
	}
}
