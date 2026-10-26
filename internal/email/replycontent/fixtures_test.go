package replycontent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Fixture is one synthetic message and what an extractor must make of it.
// The parsers (UX-005 plain text, UX-006 HTML) run against these same files.
// Expected block text is the visible text with whitespace collapsed, so it does
// not pin down how a parser serializes HTML.
type Fixture struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Client      string      `json:"client"`
	Language    string      `json:"language"`
	Format      string      `json:"format"`
	Tags        []string    `json:"tags"`
	Body        string      `json:"body"`
	Expect      Expectation `json:"expect"`
	// MustSurvive phrases must be visible: inside an authored or unknown block.
	MustSurvive []string `json:"mustSurvive"`
	// MustStayQuoted phrases must appear in a quoted block and nowhere visible.
	MustStayQuoted []string `json:"mustStayQuoted"`
}

// Expectation is the expected outcome of extracting a fixture.
type Expectation struct {
	Status   Status          `json:"status"`
	Warnings []string        `json:"warnings"`
	Blocks   []ExpectedBlock `json:"blocks"`
}

// ExpectedBlock is a block's kind and its whitespace-collapsed visible text.
type ExpectedBlock struct {
	Kind BlockKind `json:"kind"`
	Text string    `json:"text"`
}

// Result builds a Result from the expectation, so it can be validated like real output.
func (e Expectation) Result() Result {
	blocks := make([]Block, len(e.Blocks))
	for i, b := range e.Blocks {
		blocks[i] = Block{ID: fmt.Sprintf("b%d", i+1), Kind: b.Kind, Format: PartText, Content: b.Text}
	}
	return Result{Version: ExtractorVersion, Status: e.Status, Blocks: blocks, Warnings: e.Warnings}
}

func loadFixtures(t *testing.T) []Fixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	sort.Strings(paths)
	fixtures := make([]Fixture, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var f Fixture
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if want := strings.TrimSuffix(filepath.Base(path), ".json"); f.Name != want {
			t.Fatalf("%s: name %q must match the file name %q", path, f.Name, want)
		}
		fixtures = append(fixtures, f)
	}
	return fixtures
}

func visibleText(blocks []ExpectedBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Kind == BlockAuthored || b.Kind == BlockUnknown {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

func quotedText(blocks []ExpectedBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Kind == BlockQuoted {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

func TestFixturesAreWellFormed(t *testing.T) {
	validClients := map[string]bool{"gmail": true, "outlook": true, "apple_mail": true, "other": true}
	validLanguages := map[string]bool{"en": true, "fi": true}
	validFormats := map[string]bool{"text": true, "html": true}

	for _, f := range loadFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			if !validClients[f.Client] {
				t.Errorf("unknown client %q", f.Client)
			}
			if !validLanguages[f.Language] {
				t.Errorf("unknown language %q", f.Language)
			}
			if !validFormats[f.Format] {
				t.Errorf("unknown format %q", f.Format)
			}
			if f.Description == "" || len(f.Tags) == 0 || strings.TrimSpace(f.Body) == "" {
				t.Error("description, tags and body are required")
			}
			if err := f.Expect.Result().Validate(); err != nil {
				t.Errorf("expected result is invalid: %v", err)
			}
		})
	}
}

// TestFixtureTextBlocksCoverTheBody checks that for plain text nothing is lost
// or invented: the expected blocks, in order, are exactly the body.
func TestFixtureTextBlocksCoverTheBody(t *testing.T) {
	for _, f := range loadFixtures(t) {
		if f.Format != "text" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			var parts []string
			for _, b := range f.Expect.Blocks {
				parts = append(parts, b.Text)
			}
			if got, want := strings.Join(parts, " "), NormalizeText(f.Body); got != want {
				t.Errorf("blocks do not cover the body\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

func TestFixtureExpectationsKeepTheRightTextVisible(t *testing.T) {
	for _, f := range loadFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			visible := visibleText(f.Expect.Blocks)
			quoted := quotedText(f.Expect.Blocks)
			for _, phrase := range f.MustSurvive {
				if !strings.Contains(visible, phrase) {
					t.Errorf("%q must stay visible but is not in an authored or unknown block", phrase)
				}
			}
			for _, phrase := range f.MustStayQuoted {
				if strings.Contains(visible, phrase) {
					t.Errorf("%q is quoted history but would be shown as new text", phrase)
				}
				if !strings.Contains(quoted, phrase) {
					t.Errorf("%q should be in a quoted block", phrase)
				}
			}
		})
	}
}

// TestFixtureCorpusCoverage keeps the corpus honest: it must cover the clients,
// languages, layouts and failure cases the plan calls for, so a parser cannot
// pass by handling only the easy ones.
func TestFixtureCorpusCoverage(t *testing.T) {
	fixtures := loadFixtures(t)
	tags, clients, statuses := map[string]bool{}, map[string]bool{}, map[Status]bool{}
	formats := map[string]bool{}
	finnish := map[string]bool{} // clients that have a Finnish fixture
	for _, f := range fixtures {
		for _, tag := range f.Tags {
			tags[tag] = true
		}
		clients[f.Client] = true
		statuses[f.Expect.Status] = true
		formats[f.Format] = true
		if f.Language == "fi" {
			finnish[f.Client] = true
		}
	}

	for _, tag := range []string{
		"new_message", "top_reply", "bottom_reply", "inline_reply", "signature",
		"table", "code", "quote_only", "malformed", "forwarded", "nested_quote",
	} {
		if !tags[tag] {
			t.Errorf("no fixture is tagged %q", tag)
		}
	}
	for _, client := range []string{"gmail", "outlook", "apple_mail", "other"} {
		if !clients[client] {
			t.Errorf("no fixture for client %q", client)
		}
	}
	for _, client := range []string{"gmail", "outlook"} {
		if !finnish[client] {
			t.Errorf("no Finnish fixture for %s", client)
		}
	}
	for _, status := range []Status{StatusExtracted, StatusPartial, StatusFallback} {
		if !statuses[status] {
			t.Errorf("no fixture expects status %q", status)
		}
	}
	if !formats["text"] || !formats["html"] {
		t.Error("the corpus needs both text and html fixtures")
	}
}
