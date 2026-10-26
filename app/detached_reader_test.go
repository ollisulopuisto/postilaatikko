package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hkdb/aerion/internal/ipc"
)

type readerTestClient struct {
	request  ipc.Message
	response ipc.Message
}

func (c *readerTestClient) Connect(context.Context, string) error { return nil }
func (c *readerTestClient) Close() error                          { return nil }
func (c *readerTestClient) OnMessage(func(ipc.Message))           {}
func (c *readerTestClient) Send(ipc.Message) error                { return nil }
func (c *readerTestClient) SendAndWait(_ context.Context, msg ipc.Message) (*ipc.Message, error) {
	c.request = msg
	return &c.response, nil
}

func TestReaderCallPreservesArgumentsAndReportsParentError(t *testing.T) {
	client := &readerTestClient{response: ipc.Message{Payload: json.RawMessage(`{"subject":"Thread"}`)}}
	reader := NewReaderApp(ReaderConfig{ThreadID: "thread", FolderID: "folder"})
	reader.ctx = context.Background()
	reader.client = client
	close(reader.ready)
	result, err := reader.Call("GetConversation", json.RawMessage(`["thread","folder"]`))
	if err != nil || result.(map[string]any)["subject"] != "Thread" {
		t.Fatalf("result %v, %v", result, err)
	}
	var request struct {
		Operation string   `json:"operation"`
		Args      []string `json:"args"`
	}
	if err := client.request.ParsePayload(&request); err != nil {
		t.Fatal(err)
	}
	if request.Operation != "GetConversation" || len(request.Args) != 2 || request.Args[1] != "folder" {
		t.Fatalf("request %+v", request)
	}
	client.response = ipc.Message{Error: "parent unavailable"}
	if _, err = reader.Call("GetConversation", json.RawMessage(`["thread","folder"]`)); err == nil || err.Error() != "parent unavailable" {
		t.Fatal(err)
	}
	reader.startupError = errors.New("authentication failed")
	if _, err = reader.GetConfig(); err == nil {
		t.Fatal("failed authentication ignored")
	}
}

func TestReaderAllowlistExcludesAccountAndQueueAdministration(t *testing.T) {
	methods := (&App{}).readerMethods()
	for _, name := range []string{"RemoveAccount", "UpdateAccount", "SaveUIState", "QuitApp", "InitializeSendLaterClient"} {
		if _, ok := methods[name]; ok {
			t.Fatalf("unexpected reader operation %s", name)
		}
	}
	if _, ok := methods["GetReplyContent"]; !ok {
		t.Fatal("extraction unavailable")
	}
}
