package database

import "testing"

func TestGmailBodyMigrationRepairsOnlyEmptyNativeRows(t *testing.T) {
	db := openUnmigratedTestDB(t)
	applyMigrationsThrough(t, db, 54)
	for _, sql := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','Mail','a@b.c','imap.gmail.com','smtp.gmail.com','a')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','INBOX','inbox')`,
		`INSERT INTO messages(id,account_id,folder_id,uid,date,body_text,body_html,body_fetched) VALUES('empty','a','f',1,CURRENT_TIMESTAMP,'','',1),('body','a','f',2,CURRENT_TIMESTAMP,'keep','',1),('legacy','a','f',3,CURRENT_TIMESTAMP,'','',1),('crypto','a','f',4,CURRENT_TIMESTAMP,'','',1)`,
		`INSERT INTO gmail_message_refs(account_id,folder_id,provider_id,message_id) VALUES('a','f','p1','empty'),('a','f','p2','body'),('a','f','p3','crypto')`,
		`UPDATE messages SET smime_raw_body=X'01' WHERE id='crypto'`,
	} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"empty": false, "body": true, "legacy": true, "crypto": true} {
		var got bool
		if err := db.QueryRow(`SELECT body_fetched FROM messages WHERE id=?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s fetched=%v want %v", id, got, want)
		}
	}
}
