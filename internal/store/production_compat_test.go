package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Optional release preflight. Point only at a protected disposable SQLite backup,
// never the active database. No data or credentials are printed by this test.
func TestProductionSnapshotCompatibility(t *testing.T) {
	path := os.Getenv("RVS_COMPAT_SNAPSHOT")
	if path == "" {
		t.Skip("set RVS_COMPAT_SNAPSHOT to a disposable database backup")
	}
	snapshot := func(db *sql.DB) map[string][32]byte {
		t.Helper()
		names, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
		if err != nil {
			t.Fatal("cannot read database schema")
		}
		var tables []string
		for names.Next() {
			var name string
			if names.Scan(&name) != nil {
				t.Fatal("invalid table")
			}
			tables = append(tables, name)
		}
		names.Close()
		out := map[string][32]byte{}
		for _, name := range tables {
			// Names come from SQLite's own schema; quote them as identifiers.
			quoted := ""
			for _, c := range name {
				if c == '"' {
					quoted += "\"\""
				} else {
					quoted += string(c)
				}
			}
			rows, err := db.Query(`SELECT * FROM "` + quoted + `" ORDER BY rowid`)
			if err != nil {
				t.Fatal("cannot read snapshot rows")
			}
			cols, _ := rows.Columns()
			var content [][]any
			for rows.Next() {
				values := make([]any, len(cols))
				pointers := make([]any, len(cols))
				for i := range values {
					pointers[i] = &values[i]
				}
				if rows.Scan(pointers...) != nil {
					t.Fatal("cannot decode snapshot row")
				}
				content = append(content, values)
			}
			if rows.Err() != nil {
				t.Fatal("snapshot iteration failed")
			}
			rows.Close()
			encoded, _ := json.Marshal(content)
			out[name] = sha256.Sum256(encoded)
		}
		return out
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal("cannot open snapshot")
	}
	before := snapshot(raw)
	raw.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal("snapshot migration failed")
	}
	defer db.Close()
	var check string
	if err = db.db.QueryRow(`PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatal("snapshot integrity check failed")
	}
	rows, err := db.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal("foreign key check failed")
	}
	invalid := rows.Next()
	rows.Close()
	if invalid {
		t.Fatal("snapshot contains broken foreign keys")
	}
	after := snapshot(db.db)
	for table, hash := range before {
		if !reflect.DeepEqual(hash, after[table]) {
			t.Errorf("migration changed existing rows in %s", table)
		}
	}
	// Exercise the production invitation date/counter decoding, without leaking hashes.
	invites, err := db.db.Query(`SELECT id FROM upload_invitations`)
	if err != nil {
		t.Fatal("invitation schema is incompatible")
	}
	var ids []string
	for invites.Next() {
		var id string
		invites.Scan(&id)
		ids = append(ids, id)
	}
	invites.Close()
	for _, id := range ids {
		if _, err = db.UploadInvitation(context.Background(), id); err != nil {
			t.Fatal("cannot decode existing invitation")
		}
	}
}
