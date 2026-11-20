package message

import (
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"
)

func TestNeedsMeIsUnreadOrLaterWithinRealScope(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "needs-me.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','A','a@example.test','imap','smtp','a'),('b','B','b@example.test','imap','smtp','b')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('fa','a','Inbox','INBOX','inbox'),('fb','b','Work','Work','normal')`,
		`INSERT INTO messages(id,account_id,folder_id,uid,thread_id,subject,date,is_read,is_starred) VALUES('u','a','fa',1,'unread','Unread',1,0,0),('l','a','fa',2,'later','Read Later',2,1,1),('both','a','fa',3,'both','Both',3,0,1),('done','a','fa',4,'done','Done',4,1,0),('other','b','fb',1,'later','Other account same ID',5,1,0),('sleep','a','fa',5,'sleep','Snoozed',6,0,0)`,
		`INSERT INTO read_later_queue(account_id,conversation_id) VALUES('a','later'),('a','both')`,
		`INSERT INTO reply_later_queue(account_id,conversation_id) VALUES('a','later')`,
		`INSERT INTO snoozed_conversations(account_id,conversation_id,wake_at) VALUES('a','sleep','2999-01-01T00:00:00Z')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := NewStore(db)
	for _, tc := range []struct {
		filter string
		want   int
	}{{"needsMe", 3}, {"needsMe+starred", 2}, {"needsMe+unread", 2}} {
		items, err := s.ListConversationsByFolderIDs([]string{"fa", "fb"}, 0, 50, "newest", tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		count, err := s.CountConversationsByFolderIDs([]string{"fa", "fb"}, tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != tc.want || count != tc.want {
			t.Fatalf("%s list=%d count=%d want=%d", tc.filter, len(items), count, tc.want)
		}
		for _, item := range items {
			if item.AccountID != "a" {
				t.Fatalf("queue account leaked: %+v", item)
			}
		}
	}
	if _, err = db.Exec(`UPDATE messages SET is_read=1 WHERE id='u'`); err != nil {
		t.Fatal(err)
	}
	count, err := s.CountConversationsByFolderIDs([]string{"fa"}, "needsMe")
	if err != nil || count != 2 {
		t.Fatalf("read completion=%d %v", count, err)
	}
	if _, err = db.Exec(`UPDATE messages SET is_read=1 WHERE id='both'`); err != nil {
		t.Fatal(err)
	}
	count, err = s.CountConversationsByFolderIDs([]string{"fa"}, "needsMe")
	if err != nil || count != 2 {
		t.Fatalf("reading cleared Later: %d %v", count, err)
	}
}
