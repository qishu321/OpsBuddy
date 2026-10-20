package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyMigrationPreservesFilesAndIDs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "files"), 0700); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(root, "files", "original.txt")
	if err := os.WriteFile(assetPath, []byte("原成果不变"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := defaults()
	legacy.Models = []Model{{ID: "model-1", Secret: "opaque-dpapi-ciphertext"}}
	legacy.Assets = []Asset{{ID: "asset-1", ProjectID: "inbox", Path: "files/original.txt"}}
	legacy.Tasks = []Task{{ID: "task-1", Status: "succeeded", OutputIDs: []string{"asset-1"}}, {ID: "task-2", Status: "queued"}}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	legacyPath := filepath.Join(root, "state.json")
	if err := os.WriteFile(legacyPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	if a.state.Models[0].Secret != legacy.Models[0].Secret || a.state.Tasks[0].ID != "task-1" || a.state.Tasks[0].Config != nil || a.state.Tasks[1].Status != "interrupted" {
		t.Fatal("migration changed identities, credential ciphertext, or invented history")
	}
	got, _ := os.ReadFile(legacyPath)
	asset, _ := os.ReadFile(assetPath)
	if !bytes.Equal(got, raw) || string(asset) != "原成果不变" {
		t.Fatal("migration modified legacy data")
	}
	a.Close()
	header, err := os.ReadFile(filepath.Join(root, "state.db"))
	if err != nil || !bytes.HasPrefix(header, []byte("SQLite format 3\x00")) {
		t.Fatal("not a real SQLite database", err)
	}
	// Once the migration commits, stale legacy JSON must never be imported again.
	if err = os.WriteFile(legacyPath, []byte("invalid stale legacy data"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.state.Tasks) != 2 || reopened.state.Assets[0].ID != "asset-1" {
		t.Fatal("database did not remain authoritative")
	}
}

func TestInvalidLegacyMigrationCanBeRetried(t *testing.T) {
	for _, raw := range []string{`{broken`, `{}`, `null`, `{"projects":[],"roles":[],"models":[],"extensions":[],"assets":[],"tasks":[{"id":"x"},{"id":"x"}]}`} {
		t.Run(raw, func(t *testing.T) {
			root := t.TempDir()
			legacyPath := filepath.Join(root, "state.json")
			if err := os.WriteFile(legacyPath, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if a, err := New(root, t.TempDir()); err == nil {
				a.Close()
				t.Fatal("invalid migration accepted")
			}
			got, _ := os.ReadFile(legacyPath)
			if string(got) != raw {
				t.Fatal("failed migration rewrote source")
			}
			fixed, _ := json.Marshal(defaults())
			if err := os.WriteFile(legacyPath, fixed, 0600); err != nil {
				t.Fatal(err)
			}
			a, err := New(root, t.TempDir())
			if err != nil {
				t.Fatal("migration retry failed", err)
			}
			a.Close()
		})
	}
}

func TestSQLiteRejectsFutureAndCorruptDatabases(t *testing.T) {
	for _, future := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "state.db")
		if future {
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec("PRAGMA user_version=99")
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("corrupt database"), 0600); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)
		if a, err := New(root, t.TempDir()); err == nil {
			a.Close()
			t.Fatal("unsupported database accepted")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("unsupported database changed")
		}
	}
}

func TestSQLiteWriteFailureRollsBackMemoryAndDisk(t *testing.T) {
	a := newTestApp(t)
	before := a.snapshot()
	ctx := context.Background()
	// Inject a database failure at the storage boundary, not a mocked save method.
	if _, err := a.store.conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	w := request(t, a, "POST", "/api/projects", map[string]string{"name": "must-not-persist"})
	if w.Code != 500 || !reflect.DeepEqual(before, a.snapshot()) {
		t.Fatal("failed mutation remained visible", w.Code)
	}
	w = request(t, a, "POST", "/api/tasks", map[string]string{"projectId": "inbox", "prompt": "must-not-run", "kind": "docx"})
	if w.Code != 500 || len(a.cancels) != 0 || !reflect.DeepEqual(before, a.snapshot()) {
		t.Fatal("failed task submission was scheduled", w.Code)
	}
	if _, err := a.store.conn.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	// Abort a multi-entity save and verify no partial entity reaches SQLite.
	if _, err := a.store.conn.ExecContext(ctx, `CREATE TRIGGER reject_project BEFORE INSERT ON entities WHEN NEW.kind='projects' AND NEW.id='reject' BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.state.Projects = append(a.state.Projects, Project{ID: "reject"})
	a.state.Models = append(a.state.Models, Model{ID: "rollback-model"})
	err := a.saveLocked()
	a.mu.Unlock()
	if err == nil || !reflect.DeepEqual(before, a.snapshot()) {
		t.Fatal("transaction rollback failed")
	}
	disk, err := a.store.load()
	if err != nil || len(disk.Projects) != 1 || len(disk.Models) != 0 {
		t.Fatal("partial transaction persisted", err)
	}
	w = request(t, a, "POST", "/api/projects", map[string]string{"name": "persisted"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	a.Close()
	reopened, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.state.Projects) != 2 || reopened.state.Projects[1].Name != "persisted" {
		t.Fatal("failed data leaked into later commit")
	}
}

func TestSQLiteProcessHelper(t *testing.T) {
	mode := os.Getenv("OPSBUDDY_TEST_STORE_MODE")
	if mode == "" {
		return
	}
	a, err := New(os.Getenv("OPSBUDDY_TEST_STORE_ROOT"), "")
	if mode == "locked" {
		if err == nil {
			a.Close()
			os.Exit(2)
		}
		if !strings.Contains(err.Error(), "SQLITE_BUSY") {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if err != nil {
		os.Exit(4)
	}
	a.state.Tasks = append(a.state.Tasks, Task{ID: "crashed", Status: "running"})
	if err = a.saveLocked(); err != nil {
		os.Exit(5)
	}
	// Deliberately bypass Close: committed data and lock release must survive exit.
	os.Exit(0)
}

func runStoreHelper(t *testing.T, root, mode string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSQLiteProcessHelper$")
	cmd.Env = append(os.Environ(), "OPSBUDDY_TEST_STORE_MODE="+mode, "OPSBUDDY_TEST_STORE_ROOT="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper %s: %v %s", mode, err, out)
	}
}

func TestSQLiteSingleProcessAndCrashRecovery(t *testing.T) {
	a := newTestApp(t)
	runStoreHelper(t, a.root, "locked")
	a.Close()
	runStoreHelper(t, a.root, "crash")
	recovered, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if len(recovered.state.Tasks) != 1 || recovered.state.Tasks[0].Status != "interrupted" {
		t.Fatal("abrupt exit lost committed task or recovery state")
	}
	var integrity string
	if err := recovered.store.conn.QueryRowContext(context.Background(), "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("database integrity", integrity, err)
	}
}
