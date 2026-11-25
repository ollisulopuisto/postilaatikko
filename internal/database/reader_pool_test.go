package database

import (
	"path/filepath"
	"testing"
)

func TestReaderPoolIsReadOnlyAndSeesCommittedWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Reader() == db.DB {
		t.Fatal("Reader must be a separate pool")
	}
	if got := db.Reader().Stats().MaxOpenConnections; got != ReaderMaxOpenConns {
		t.Fatalf("reader pool max open = %d, want %d", got, ReaderMaxOpenConns)
	}
	if _, err := db.Exec("CREATE TABLE t (v INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := db.Reader().QueryRow("SELECT v FROM t").Scan(&v); err != nil || v != 7 {
		t.Fatalf("reader read = %d, %v", v, err)
	}
	if _, err := db.Reader().Exec("INSERT INTO t VALUES (8)"); err == nil {
		t.Fatal("reader pool accepted a write")
	}
	var mode string
	if err := db.Reader().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("reader journal mode = %q, %v", mode, err)
	}
}
