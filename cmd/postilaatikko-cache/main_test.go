package main

import (
	"fmt"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRollbackCommandRefusesMissingOrWrongDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.db")
	if err := run([]string{"--db", path, "--account", "a", "--message", "m"}); err == nil {
		t.Fatal("created missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("typo created a file")
	}
	if err := run(nil); err == nil {
		t.Fatal("missing ownership accepted")
	}
}

func TestRollbackCommandRestoresBackedUpCache(t *testing.T) {
	for _, version := range []int{79, 85, 86} {
		t.Run(fmt.Sprint(version), func(t *testing.T) { testRollbackSchema(t, version) })
	}
}

func testRollbackSchema(t *testing.T, version int) {
	path := filepath.Join(t.TempDir(), "mail.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	applyHistoricalMigrations(t, db, version)
	const schemaQuery = `SELECT group_concat(name || ':' || COALESCE(sql,''), char(10)) FROM (SELECT name,sql FROM sqlite_master ORDER BY type,name)`

	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','A','a@example.test','imap','smtp','a')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','Inbox','inbox')`); err != nil {
		t.Fatal(err)
	}
	// Seed as a historical app would, without current-store retention hooks.
	if _, err = db.Exec(`INSERT INTO messages(id,account_id,folder_id,uid,thread_id,date,received_at) VALUES('mail','a','f',1,'mail',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	store := message.NewAttachmentStore(db)
	if err = store.Create(&message.Attachment{ID: "legacy", MessageID: "mail", Filename: "old", Size: 3}); err != nil {
		t.Fatal(err)
	}
	backup, err := store.RepairFromMIME("a", "mail", "digest", []message.RepairPart{{Metadata: message.Attachment{ID: "new", MessageID: "mail", Filename: "new", Size: 3}, Payload: []byte("one")}}, "")
	if err != nil {
		t.Fatal(err)
	}

	var originalSchema string
	if err = db.QueryRow(schemaQuery).Scan(&originalSchema); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"--db", path, "--account", "a", "--message", "mail"}
	if err = run(args); err != nil {
		t.Fatal(err)
	}
	if err = run(append(args, "--restore", backup)); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := message.NewAttachmentStore(db).GetByMessage("mail")
	if err != nil || len(got) != 1 || got[0].ID != "legacy" {
		t.Fatalf("CLI restore lost original: %v %v", got, err)
	}
	next, err := message.NewAttachmentStore(db).RepairBackupID("mail")
	if err != nil || next == "" || next == backup {
		t.Fatal("missing reversible generation", next, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = run(append(args, "--restore", next)); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = message.NewAttachmentStore(db).GetByMessage("mail")
	if err != nil || len(got) != 1 || got[0].ID != "new" {
		t.Fatal("redo lost repaired state", got, err)
	}
	var finalSchema string
	var finalVersion int
	if err = db.QueryRow(schemaQuery).Scan(&finalSchema); err != nil || finalSchema != originalSchema {
		t.Fatal("rollback changed historical schema", err)
	}
	if err = db.QueryRow(`SELECT MAX(version) FROM migrations`).Scan(&finalVersion); err != nil || finalVersion != version {
		t.Fatal("rollback migrated database", finalVersion, err)
	}
	if version < 86 {
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='attachment_download_integrity'`).Scan(&count); err != nil || count != 0 {
			t.Fatal("offline rollback changed schema", count, err)
		}
	}
}

// Apply exactly the repository's migration SQL through the requested version,
// not a latest-schema database with selected objects removed. Parse the literal
// migration list here so this CLI test does not add a downgrade API to production.
func applyHistoricalMigrations(t *testing.T, db *database.DB, version int) {
	t.Helper()
	source, err := parser.ParseFile(token.NewFileSet(), "../../internal/database/migrations.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	applyHistoricalMigrationSource(t, db, version, source)
}

func applyHistoricalMigrationSource(t *testing.T, db *database.DB, version int, source *ast.File) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	applied := 0
	ast.Inspect(source, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "migrations" {
			return true
		}
		list, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("migration list is not literal")
		}
		for _, element := range list.Elts {
			entry, ok := element.(*ast.CompositeLit)
			if !ok {
				t.Fatal("migration entry is not literal")
			}
			// Inspect the version first. SQL after the cutoff is irrelevant,
			// even when its expression no longer uses a string literal.
			fields := map[string]ast.Expr{}
			for _, field := range entry.Elts {
				pair, ok := field.(*ast.KeyValueExpr)
				if !ok {
					t.Fatal("migration field is not keyed")
				}
				key, ok := pair.Key.(*ast.Ident)
				if !ok {
					t.Fatal("migration key is not an identifier")
				}
				fields[key.Name] = pair.Value
			}
			versionValue, ok := fields["Version"].(*ast.BasicLit)
			if !ok {
				t.Fatal("migration version is not literal")
			}
			number, err := strconv.Atoi(versionValue.Value)
			if err != nil {
				t.Fatal(err)
			}
			if number > version {
				break
			}
			sqlValue, ok := fields["SQL"].(*ast.BasicLit)
			if !ok {
				t.Fatal("historical SQL is not literal")
			}
			sqlText, err := strconv.Unquote(sqlValue.Value)
			if err != nil {
				t.Fatal(err)
			}
			if number <= applied || sqlText == "" {
				t.Fatal("invalid migration fixture", number)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(sqlText); err == nil {
				_, err = tx.Exec(`INSERT INTO migrations(version) VALUES(?)`, number)
			}
			if err != nil {
				_ = tx.Rollback()
				t.Fatalf("migration %d: %v", number, err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			applied = number
		}
		return false
	})
	if applied != version {
		t.Fatalf("applied migrations through %d, wanted %d", applied, version)
	}
}

func TestHistoricalFixtureIgnoresFutureSQLExpression(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
 const futureSQL = "invalid future SQL must not run"
 var migrations = []Migration{
  {Version:1, SQL:"CREATE TABLE historical (id TEXT)"},
  {SQL:futureSQL, Version:2},
 }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(t.TempDir(), "historical.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyHistoricalMigrationSource(t, db, 1, source)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='historical'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("historical migration missing", count, err)
	}
}
