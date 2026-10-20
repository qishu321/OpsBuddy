package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// A dedicated connection retains SQLite's exclusive lock for the application's
// lifetime. Two processes must never cache and modify the same workspace state.
type stateStore struct {
	db   *sql.DB
	conn *sql.Conn
}

func openStore(root string) (*stateStore, State, error) {
	db, err := sql.Open("sqlite", filepath.Join(root, "state.db"))
	if err != nil {
		return nil, State{}, err
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, State{}, err
	}
	s := &stateStore{db: db, conn: conn}
	state, err := s.initialize(root)
	if err != nil {
		s.close()
		return nil, State{}, fmt.Errorf("本地数据库无法打开（请确认同一数据目录没有其他实例运行）；已保留原数据：%w", err)
	}
	return s, state, nil
}

func (s *stateStore) initialize(root string) (State, error) {
	ctx := context.Background()
	for _, pragma := range []string{"PRAGMA busy_timeout=1000", "PRAGMA locking_mode=EXCLUSIVE", "PRAGMA journal_mode=DELETE", "PRAGMA synchronous=FULL"} {
		if _, err := s.conn.ExecContext(ctx, pragma); err != nil {
			return State{}, err
		}
	}
	if _, err := s.conn.ExecContext(ctx, "BEGIN EXCLUSIVE; COMMIT;"); err != nil {
		return State{}, err
	}
	var version int
	if err := s.conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return State{}, err
	}
	if version > 4 {
		return State{}, errors.New("数据库来自更新版本，请使用兼容版本打开")
	}
	if version == 4 {
		return s.load()
	}
	if version >= 1 && version <= 3 {
		state, err := s.load()
		if err != nil {
			return State{}, err
		}
		if _, err = s.conn.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
			return State{}, err
		}
		return state, nil
	}
	var tables int
	if err := s.conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return State{}, err
	}
	if tables != 0 {
		return State{}, errors.New("数据库结构未知，拒绝覆盖")
	}
	state := defaults()
	raw, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err == nil {
		// Never rewrite, rename or remove the legacy file. Migration is committed
		// with its schema version; a failed transaction can be retried safely.
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err != nil {
			return State{}, errors.New("旧版 state.json 无法解析")
		}
		for _, name := range []string{"projects", "roles", "models", "extensions", "assets", "tasks"} {
			if _, ok := fields[name]; !ok {
				return State{}, fmt.Errorf("旧版 state.json 缺少 %s", name)
			}
		}
		if err = json.Unmarshal(raw, &state); err != nil {
			return State{}, errors.New("旧版 state.json 结构无效")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	records, err := stateRecords(state)
	if err != nil {
		return State{}, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE entities (
		kind TEXT NOT NULL, id TEXT NOT NULL, ordinal INTEGER NOT NULL,
		payload TEXT NOT NULL CHECK(json_valid(payload)), PRIMARY KEY(kind, id)
	)`); err != nil {
		return State{}, err
	}
	for _, record := range records {
		if err = writeRecord(tx, record); err != nil {
			return State{}, err
		}
	}
	if _, err = tx.Exec("PRAGMA user_version=4"); err != nil {
		return State{}, err
	}
	if err = tx.Commit(); err != nil {
		return State{}, err
	}
	return state, nil
}

type stateRecord struct {
	kind, id, payload string
	ordinal           int
}

func stateRecords(state State) (map[string]stateRecord, error) {
	state.StartupImport = nil // response-only metadata is not an entity collection
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var groups map[string][]json.RawMessage
	if err = json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	records := map[string]stateRecord{}
	for kind, values := range groups {
		for ordinal, value := range values {
			var entity struct {
				ID string `json:"id"`
			}
			if err = json.Unmarshal(value, &entity); err != nil || entity.ID == "" {
				return nil, fmt.Errorf("%s 包含无效 ID", kind)
			}
			key := kind + "\x00" + entity.ID
			if _, exists := records[key]; exists {
				return nil, fmt.Errorf("%s 包含重复 ID", kind)
			}
			records[key] = stateRecord{kind: kind, id: entity.ID, ordinal: ordinal, payload: string(value)}
		}
	}
	return records, nil
}

func writeRecord(tx *sql.Tx, r stateRecord) error {
	_, err := tx.Exec(`INSERT INTO entities(kind,id,ordinal,payload) VALUES(?,?,?,?)
		ON CONFLICT(kind,id) DO UPDATE SET ordinal=excluded.ordinal,payload=excluded.payload`, r.kind, r.id, r.ordinal, r.payload)
	return err
}

func (s *stateStore) load() (State, error) {
	rows, err := s.conn.QueryContext(context.Background(), "SELECT kind,id,payload FROM entities ORDER BY kind,ordinal")
	if err != nil {
		return State{}, err
	}
	defer rows.Close()
	groups := map[string][]json.RawMessage{}
	for _, kind := range []string{"projects", "roles", "models", "extensions", "assets", "tasks", "knowledgeBases", "knowledgeDocuments", "workflows", "workflowRuns", "agentGroups", "memories", "schedules", "scheduleRuns", "directories", "notifications"} {
		groups[kind] = []json.RawMessage{}
	}
	for rows.Next() {
		var kind, id, payload string
		if err = rows.Scan(&kind, &id, &payload); err != nil {
			return State{}, err
		}
		var entity struct {
			ID string `json:"id"`
		}
		if _, ok := groups[kind]; !ok || json.Unmarshal([]byte(payload), &entity) != nil || entity.ID != id {
			return State{}, errors.New("数据库记录结构无效")
		}
		groups[kind] = append(groups[kind], json.RawMessage(payload))
	}
	if err = rows.Err(); err != nil {
		return State{}, err
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return State{}, err
	}
	var state State
	err = json.Unmarshal(raw, &state)
	return state, err
}

func (s *stateStore) save(before, after State) error {
	oldRecords, err := stateRecords(before)
	if err != nil {
		return err
	}
	records, err := stateRecords(after)
	if err != nil {
		return err
	}
	tx, err := s.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, record := range records {
		if old, ok := oldRecords[key]; ok && old == record {
			continue
		}
		if err = writeRecord(tx, record); err != nil {
			return err
		}
	}
	for key, record := range oldRecords {
		if _, ok := records[key]; !ok {
			if _, err = tx.Exec("DELETE FROM entities WHERE kind=? AND id=?", record.kind, record.id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *stateStore) close() {
	_ = s.conn.Close()
	_ = s.db.Close()
}
