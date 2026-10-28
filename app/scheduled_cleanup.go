package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/hkdb/aerion/internal/gmail"
	"io"
	"net/textproto"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/hkdb/aerion/internal/draft"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/scheduledsend"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func cleanupScheduledDraft(ctx context.Context, data string, job scheduledsend.ClientJob, ops *draftOps, cancelSync func(string), clearHistory func(accountID, draftID string)) scheduledsend.ClientJob {
	if job.State != "sent" || job.DraftCleanup == "removed" || job.DraftCleanup == "kept_changed" || job.DraftCleanup == "manual_legacy" {
		return job
	}
	store, err := clientStore(data)
	if err != nil {
		job.DraftCleanup = "retry"
		return job
	}
	defer store.Close()
	finish := func(state string) scheduledsend.ClientJob {
		if err := store.SetDraftCleanup(ctx, job.ID, state); err != nil {
			job.DraftCleanup = "retry"
			return job
		}
		// Actual send ended the draft: its undo history ends too (approved
		// lifecycle). Best-effort: a clear failure must not flip the job to
		// retry, the draft row is already gone.
		if state == "removed" && clearHistory != nil {
			clearHistory(job.AccountID, job.DraftID)
		}
		job.DraftCleanup = state
		return job
	}
	if job.Code == "sent_copy_failed" {
		return finish("waiting_sent_copy")
	}
	if job.DraftFingerprint == "" {
		return finish("manual_legacy")
	}
	cancelSync(job.DraftID)
	current, err := ops.draftStore.Get(job.DraftID)
	if err != nil {
		return finish("retry")
	}
	if current == nil {
		return finish("removed")
	}
	if current.AccountID != job.AccountID || draft.Fingerprint(current) != job.DraftFingerprint {
		return finish("kept_changed")
	}
	cleanupContext, done := context.WithTimeout(ctx, 25*time.Second)
	defer done()
	if ops.gmailClient != nil {
		c, apiErr := ops.gmailClient(cleanupContext, current.AccountID)
		if apiErr == nil {
			provider, err := ops.gmailDraftID(cleanupContext, c, current)
			if err != nil && !errors.Is(err, gmail.ErrNotFound) {
				return finish("retry")
			}
			if provider != "" {
				raw, err := c.DraftRaw(cleanupContext, provider)
				if err != nil && !errors.Is(err, gmail.ErrNotFound) {
					return finish("retry")
				}
				if err == nil {
					reader := bufio.NewReader(bytes.NewReader(raw))
					line1, _ := reader.ReadString('\n')
					line2, _ := reader.ReadString('\n')
					line3, _ := reader.ReadString('\n')
					if line1 != "X-Postilaatikko-Draft-ID: "+current.ID+"\r\n" || line2 != "X-Postilaatikko-Draft-Source: "+job.DraftFingerprint+"\r\n" || !strings.HasPrefix(line3, "X-Postilaatikko-Draft-Hash: ") {
						return finish("manual_legacy")
					}
					expected := strings.TrimSpace(strings.TrimPrefix(line3, "X-Postilaatikko-Draft-Hash: "))
					remainder, _ := io.ReadAll(reader)
					actual := sha256.Sum256(remainder)
					if expected != hex.EncodeToString(actual[:]) {
						return finish("kept_changed")
					}
					if err = c.DeleteDraft(cleanupContext, provider); err != nil && !errors.Is(err, gmail.ErrNotFound) {
						return finish("retry")
					}
				}
			}
			deleted, err := ops.draftStore.DeleteUnchanged(cleanupContext, current)
			if err != nil {
				return finish("retry")
			}
			if !deleted {
				return finish("kept_changed")
			}
			return finish("removed")
		} else if !errors.Is(apiErr, gmail.ErrNotCapable) {
			return finish("retry")
		}
	}

	var draftsFolder *folder.Folder
	if current.IsSynced() {
		if current.FolderID == "" {
			return finish("manual_legacy")
		}
		draftsFolder, err = ops.folderStore.Get(current.FolderID)
		if err != nil || draftsFolder == nil || draftsFolder.AccountID != current.AccountID {
			return finish("retry")
		}
		poolConnection, err := ops.imapPool.GetConnection(cleanupContext, current.AccountID)
		if err != nil {
			return finish("retry")
		}
		defer ops.imapPool.Release(poolConnection)
		connection := poolConnection.Client()
		finished := make(chan struct{})
		defer close(finished)
		go func() {
			select {
			case <-cleanupContext.Done():
				_ = connection.ForceClose()
			case <-finished:
			}
		}()
		selected, err := connection.SelectMailbox(cleanupContext, draftsFolder.Path)
		if err != nil {
			return finish("retry")
		}
		if draftsFolder.UIDValidity != 0 && selected.UIDValidity != draftsFolder.UIDValidity {
			return finish("manual_legacy")
		}
		matches, err := remoteDraftMatches(connection.RawClient(), goimap.UID(current.IMAPUID), current.ID, job.DraftFingerprint)
		if err != nil {
			return finish("retry")
		}
		if !matches {
			return finish("manual_legacy")
		}
		if err := connection.DeleteMessageByUID(goimap.UID(current.IMAPUID)); err != nil {
			return finish("retry")
		}
		if err := ops.messageStore.DeleteByUID(draftsFolder.ID, current.IMAPUID); err != nil {
			return finish("retry")
		}
	}
	deleted, err := ops.draftStore.DeleteUnchanged(cleanupContext, current)
	if err != nil {
		return finish("retry")
	}
	if !deleted {
		return finish("kept_changed")
	}
	if draftsFolder != nil {
		wailsRuntime.EventsEmit(ctx, "messages:updated", map[string]interface{}{"accountId": current.AccountID, "folderId": draftsFolder.ID})
	}
	return finish("removed")
}

func remoteDraftMatches(client *imapclient.Client, uid goimap.UID, id, source string) (bool, error) {
	section := &goimap.FetchItemBodySection{Specifier: goimap.PartSpecifierHeader, HeaderFields: []string{"X-Postilaatikko-Draft-ID", "X-Postilaatikko-Draft-Source", "X-Postilaatikko-Draft-Hash"}, Peek: true,
		Partial: &goimap.SectionPartial{Offset: 0, Size: 2048}}
	command := client.Fetch(goimap.UIDSetNum(uid), &goimap.FetchOptions{UID: true, BodySection: []*goimap.FetchItemBodySection{section}})
	var header []byte
	var fetched goimap.UID
	sawMessage := false
	for message := command.Next(); message != nil; message = command.Next() {
		sawMessage = true
		for item := message.Next(); item != nil; item = message.Next() {
			switch value := item.(type) {
			case imapclient.FetchItemDataUID:
				fetched = value.UID
			case imapclient.FetchItemDataBodySection:
				var err error
				header, err = io.ReadAll(io.LimitReader(value.Literal, 2049))
				if err != nil || len(header) > 2048 {
					_ = client.Close()
					_ = command.Close()
					return false, errors.New("invalid remote draft marker size")
				}
			}
		}
	}
	if err := command.Close(); err != nil {
		return false, err
	}
	if !sawMessage {
		return true, nil
	} // The recorded UID is already gone.
	if fetched != uid || len(header) == 0 {
		return false, nil
	}
	parsed, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(header))).ReadMIMEHeader()
	if err != nil {
		return false, err
	}
	if len(parsed["X-Postilaatikko-Draft-Id"]) != 1 || parsed.Get("X-Postilaatikko-Draft-ID") != id || parsed.Get("X-Postilaatikko-Draft-Source") != source {
		return false, nil
	}
	digest := parsed.Get("X-Postilaatikko-Draft-Hash")
	if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
		return false, nil
	}
	return remoteDraftBodyMatches(client, uid, id, source, digest)
}

func remoteDraftBodyMatches(client *imapclient.Client, uid goimap.UID, id, source, digest string) (bool, error) {
	const limit = 2 * scheduledsend.MaxMessageBytes
	section := &goimap.FetchItemBodySection{Peek: true, Partial: &goimap.SectionPartial{Offset: 0, Size: limit + 1}}
	command := client.Fetch(goimap.UIDSetNum(uid), &goimap.FetchOptions{UID: true, BodySection: []*goimap.FetchItemBodySection{section}})
	matched := false
	var fetched goimap.UID
	for message := command.Next(); message != nil; message = command.Next() {
		for item := message.Next(); item != nil; item = message.Next() {
			switch value := item.(type) {
			case imapclient.FetchItemDataUID:
				fetched = value.UID
			case imapclient.FetchItemDataBodySection:
				reader := bufio.NewReaderSize(io.LimitReader(value.Literal, limit+1), 1024)
				prefix := []string{"X-Postilaatikko-Draft-ID: " + id + "\r\n", "X-Postilaatikko-Draft-Source: " + source + "\r\n", "X-Postilaatikko-Draft-Hash: " + digest + "\r\n"}
				prefixBytes := 0
				for _, expected := range prefix {
					line, err := reader.ReadSlice('\n')
					if err != nil || string(line) != expected {
						_ = client.Close()
						_ = command.Close()
						return false, nil
					}
					prefixBytes += len(line)
				}
				hash := sha256.New()
				count, err := io.Copy(hash, reader)
				if err != nil || count+int64(prefixBytes) > limit {
					_ = client.Close()
					_ = command.Close()
					return false, fmt.Errorf("remote draft content could not be verified within cleanup limits")
				}
				matched = hex.EncodeToString(hash.Sum(nil)) == digest
			}
		}
	}
	if err := command.Close(); err != nil {
		return false, err
	}
	return matched && fetched == uid, nil
}
