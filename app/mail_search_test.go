package app

import (
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/virtualfolder"
	"strings"
	"testing"
	"time"
)

func TestGmailVirtualFolderUnionAndExclusion(t *testing.T) {
	fs := []*folder.Folder{{Path: "perhe/aarni", Type: folder.TypeFolder}, {Path: "perhe/bisnes hedda", Type: folder.TypeFolder}}
	q := gmailViewQuery("is:unread", fs, false)
	if !strings.Contains(q, `label:perhe/aarni OR label:perhe/bisnes-hedda`) {
		t.Fatal(q)
	}
	excluded := gmailViewQuery("", fs, true)
	if !strings.HasPrefix(excluded, "-(") {
		t.Fatal(excluded)
	}
}
func TestGmailThreadFiltersUseLatestActivityAndWholeThreadFlags(t *testing.T) {
	items := []gmail.Message{{Date: time.Now().AddDate(0, 0, -60), LabelIDs: []string{"UNREAD"}}, {Date: time.Now(), LabelIDs: []string{"SENT"}, InReplyTo: "<parent>"}}
	vf := &virtualfolder.VirtualFolder{ReadStatus: "unread", ReplyStatus: "replied", DateWindow: "7d"}
	if !matchesGmailThread(items, vf) {
		t.Fatal("latest activity or reply not recognized")
	}
	vf.ReadStatus = "read"
	if matchesGmailThread(items, vf) {
		t.Fatal("mixed unread conversation matched read")
	}
}
