package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestListAttachmentsUsesAccountQualifiedReference(t *testing.T) {
	client := New(Config{Endpoint: "https://mail.example.test", Token: "device-token"})
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/messages/attachments/list" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer device-token" {
			t.Fatal("missing bearer authorization")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"accountId":"account-a"`) || !strings.Contains(string(body), `"messageId":"message-a"`) {
			t.Fatalf("request body = %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"items":[{"id":"att-a","filename":"image.png","contentType":"image/png","size":4,"isInline":true}],"complete":true}}`))}, nil
	})

	items, err := client.ListAttachments(context.Background(), v1.MessageRef{AccountID: "account-a", MessageID: "message-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "att-a" || !items[0].IsInline {
		t.Fatalf("attachments = %#v", items)
	}
}

func TestReadAttachmentBytesPreservesRawResponseAndContentType(t *testing.T) {
	want := []byte{0, 1, 2, 255}
	client := New(Config{Endpoint: "https://mail.example.test", Token: "device-token"})
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/messages/attachments/read" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer device-token" {
			t.Fatal("missing bearer authorization")
		}
		header := make(http.Header)
		header.Set("Content-Type", "image/png")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(string(want)))}, nil
	})

	got, contentType, err := client.ReadAttachmentBytes(context.Background(), v1.MessageRef{AccountID: "account-a", MessageID: "message-a"}, "att-a")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || contentType != "image/png" {
		t.Fatalf("response = %v, %q; want %v, image/png", got, contentType, want)
	}
}
