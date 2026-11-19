package settings

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestEmailTemplatesPersistUpdateAndDeleteWithoutTouchingOtherSettings(t *testing.T) {
	s := NewStore(openTestDB(t))
	if _, err := s.SaveQuickReply(QuickReply{Name: "short", Body: "keep"}); err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveEmailTemplate(EmailTemplate{Name: "  Greeting  ", Subject: "  Hello  ", Body: "First\nSecond"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveEmailTemplate(EmailTemplate{Name: "Body only", Body: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.ID == b.ID || a.Name != "Greeting" || a.Subject != "Hello" {
		t.Fatalf("invalid created template: %#v %#v", a, b)
	}
	a.Body = "Updated body"
	if _, err := s.SaveEmailTemplate(a); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListEmailTemplates()
	if err != nil || len(got) != 2 || got[0].Body != "Updated body" || got[1].ID != b.ID {
		t.Fatalf("update/order: %#v %v", got, err)
	}
	if err := s.DeleteEmailTemplate(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEmailTemplate("missing"); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListEmailTemplates()
	if err != nil || len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("delete: %#v %v", got, err)
	}
	snippets, err := s.ListQuickReplies()
	if err != nil || len(snippets) != 1 || snippets[0].Body != "keep" {
		t.Fatalf("changed quick replies: %#v %v", snippets, err)
	}
}

func TestEmailTemplatesRejectInvalidWritesWithoutLosingSavedTemplates(t *testing.T) {
	s := NewStore(openTestDB(t))
	saved, err := s.SaveEmailTemplate(EmailTemplate{Name: "Keep", Subject: "Subject only"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []EmailTemplate{{Name: " ", Body: "body"}, {Name: "Blank", Subject: " ", Body: "\n"}, {ID: "unknown", Name: "Wrong", Body: "body"}} {
		if _, err := s.SaveEmailTemplate(input); err == nil {
			t.Errorf("accepted invalid template: %#v", input)
		}
		got, err := s.ListEmailTemplates()
		if err != nil || len(got) != 1 || got[0].ID != saved.ID {
			t.Fatalf("invalid write changed templates: %#v %v", got, err)
		}
	}
}

func TestEmailTemplatesEmptyAndCorruptStorage(t *testing.T) {
	s := NewStore(openTestDB(t))
	got, err := s.ListEmailTemplates()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("initial list: %#v %v", got, err)
	}
	if err := s.Set(KeyEmailTemplates, "not json"); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListEmailTemplates()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("corrupt list: %#v %v", got, err)
	}
}

func TestConcurrentTemplateSavesKeepEveryTemplate(t *testing.T) {
	s := NewStore(openTestDB(t))
	const count = 12
	start := make(chan struct{})
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := s.SaveEmailTemplate(EmailTemplate{Name: fmt.Sprintf("Template %d", i), Body: "body"})
			errors <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListEmailTemplates()
	if err != nil || len(got) != count {
		t.Fatalf("lost concurrent template writes: count=%d want %d error=%v", len(got), count, err)
	}
}

func TestEmailTemplateLimitsPreserveStoredData(t *testing.T) {
	for _, input := range []EmailTemplate{
		{Name: strings.Repeat("x", 257), Body: "body"},
		{Name: "subject", Subject: strings.Repeat("x", 4097)},
		{Name: "body", Body: strings.Repeat("x", 65537)},
	} {
		t.Run(input.Name[:1], func(t *testing.T) {
			s := NewStore(openTestDB(t))
			saved, err := s.SaveEmailTemplate(EmailTemplate{Name: "keep", Body: "keep"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.SaveEmailTemplate(input); err == nil {
				t.Fatal("accepted oversized field")
			}
			got, err := s.ListEmailTemplates()
			if err != nil || len(got) != 1 || got[0] != saved {
				t.Fatalf("changed stored templates: %v %v", got, err)
			}
		})
	}
	t.Run("count", func(t *testing.T) {
		s := NewStore(openTestDB(t))
		templates := make([]EmailTemplate, 100)
		for i := range templates {
			templates[i] = EmailTemplate{ID: fmt.Sprint(i), Name: "keep", Body: "body"}
		}
		data, _ := json.Marshal(templates)
		if err := s.Set(KeyEmailTemplates, string(data)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveEmailTemplate(EmailTemplate{Name: "extra", Body: "body"}); err == nil {
			t.Fatal("accepted 101st template")
		}
		templates[0].Body = "updated"
		if _, err := s.SaveEmailTemplate(templates[0]); err != nil {
			t.Fatal("update at count limit:", err)
		}
		got, err := s.ListEmailTemplates()
		if err != nil || len(got) != 100 || got[0].Body != "updated" {
			t.Fatalf("count/update: %d %v", len(got), err)
		}
	})
}
