package replycontent

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func kindsOf(r Result) []BlockKind {
	kinds := make([]BlockKind, len(r.Blocks))
	for i, b := range r.Blocks {
		kinds[i] = b.Kind
	}
	return kinds
}

// TestExtractPlainMatchesFixtures runs every plain-text fixture of the corpus.
func TestExtractPlainMatchesFixtures(t *testing.T) {
	for _, f := range loadFixtures(t) {
		if f.Format != "text" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			got := ExtractPlain(f.Body)
			if err := got.Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
			if got.Status != f.Expect.Status {
				t.Errorf("status = %q, want %q", got.Status, f.Expect.Status)
			}
			if strings.Join(got.Warnings, ",") != strings.Join(f.Expect.Warnings, ",") {
				t.Errorf("warnings = %v, want %v", got.Warnings, f.Expect.Warnings)
			}
			if len(got.Blocks) != len(f.Expect.Blocks) {
				t.Fatalf("got %d blocks %v, want %d", len(got.Blocks), kindsOf(got), len(f.Expect.Blocks))
			}
			for i, want := range f.Expect.Blocks {
				b := got.Blocks[i]
				if b.Kind != want.Kind {
					t.Errorf("block %d kind = %q, want %q", i+1, b.Kind, want.Kind)
				}
				if text := NormalizeText(b.Content); text != want.Text {
					t.Errorf("block %d text = %q, want %q", i+1, text, want.Text)
				}
			}
		})
	}
}

// Blocks point back at the stored body and never rewrite it.
func TestExtractPlainSourcesPointAtTheOriginalBody(t *testing.T) {
	bodies := []string{
		"Tuesday works.\r\n\r\nOn Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:\r\n\r\n> Could we meet?\r\n",
		"Hi,\n\nSee below.\n\n-- \nBob\nEngineer\n",
		"Hei.\n\n> vanha\n\nJa lisää.\n",
	}
	for _, body := range bodies {
		got := ExtractPlain(body)
		if err := got.Validate(); err != nil {
			t.Fatalf("invalid result for %q: %v", body, err)
		}
		for _, b := range got.Blocks {
			if b.Source == nil || b.Source.Part != PartText {
				t.Fatalf("block %s has no text source", b.ID)
			}
			if body[b.Source.Start:b.Source.End] != b.Content {
				t.Errorf("block %s content %q is not body[%d:%d] = %q", b.ID, b.Content, b.Source.Start, b.Source.End, body[b.Source.Start:b.Source.End])
			}
		}
	}
}

func TestExtractPlainEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   Status
		kinds    []BlockKind
		warnings []string
	}{
		{"empty body", "", StatusUnavailable, nil, nil},
		{"only whitespace", " \n\t\r\n ", StatusUnavailable, nil, nil},
		{"CRLF line endings", "Yes.\r\n\r\nOn Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:\r\n> Can we?\r\n",
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"attribution wrapped over two lines",
			"Yes.\n\nOn Mon, Oct 19, 2026 at 9:00 AM Alice Example\n<alice@example.org> wrote:\n\n> Can we?\n",
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"prose that mentions wrote is not an attribution",
			"As he wrote:\nthe plan is fine.\n", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"a single From line in prose stays authored",
			"From: my side, the plan is fine.\nThanks.\n", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"signature delimiter without trailing space",
			"Done.\n\n--\nBob\n", StatusExtracted, []BlockKind{BlockAuthored, BlockSignature}, nil},
		{"signature delimiter first in the body is not trusted",
			"-- \nBob\n", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"a long block after a delimiter is not a signature",
			"Intro.\n\n-- \n" + strings.Repeat("line of real content\n", 40),
			StatusPartial, []BlockKind{BlockAuthored, BlockUnknown}, nil},
		{"signature before quoted history",
			"Done.\n\n-- \nBob\n\nOn Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:\n> Hi\n",
			StatusExtracted, []BlockKind{BlockAuthored, BlockSignature, BlockQuoted}, nil},
		{"mobile footer in the middle stays authored",
			"Sent from my iPhone is a funny default.\nSee you.\n", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"English header run without a separator line",
			"Fine.\n\nFrom: Alice <a@example.org>\nSent: Monday\nTo: Bob\nSubject: Hi\n\nBody\n",
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"forwarded message only is a fallback",
			"---------- Forwarded message ---------\nFrom: Alice <a@example.org>\nSubject: Hi\n\nBody\n",
			StatusFallback, []BlockKind{BlockUnknown}, []string{WarnForwardedContent}},
		{"quote without attribution at the end is quoted history",
			"Thanks.\n\n> earlier\n> text\n", StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"quote followed by a reply without attribution stays visible",
			"> earlier question\n\nMy answer.\n", StatusPartial, []BlockKind{BlockUnknown, BlockAuthored}, []string{WarnAmbiguousQuoteMarker}},
		{"indented quote markers",
			"Yes.\n\nOn Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:\n  > indented\n  > quote\n",
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPlain(tt.body)
			if err := got.Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
			if got.Status != tt.status {
				t.Errorf("status = %q, want %q (blocks %v)", got.Status, tt.status, kindsOf(got))
			}
			if fmt.Sprint(kindsOf(got)) != fmt.Sprint(tt.kinds) && !(len(tt.kinds) == 0 && len(got.Blocks) == 0) {
				t.Errorf("kinds = %v, want %v", kindsOf(got), tt.kinds)
			}
			if strings.Join(got.Warnings, ",") != strings.Join(tt.warnings, ",") {
				t.Errorf("warnings = %v, want %v", got.Warnings, tt.warnings)
			}
		})
	}
}

// Input far over the limit is returned whole and unanalyzed, so work stays bounded.
func TestExtractPlainIsBounded(t *testing.T) {
	body := "Tuesday works.\n\n" + strings.Repeat("> quoted line\n", maxPlainBytes/10)
	got := ExtractPlain(body)
	if err := got.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	if got.Status != StatusFallback || len(got.Blocks) != 1 || got.Blocks[0].Kind != BlockUnknown {
		t.Fatalf("oversized body: status %q, kinds %v; want a single unknown block", got.Status, kindsOf(got))
	}
	if got.Blocks[0].Content != strings.TrimSpace(body) {
		t.Error("the fallback block must hold the whole body")
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != WarnInputTooLarge {
		t.Errorf("warnings = %v, want [%s]", got.Warnings, WarnInputTooLarge)
	}
}

func TestExtractPlainIsDeterministic(t *testing.T) {
	for _, f := range loadFixtures(t) {
		if f.Format != "text" {
			continue
		}
		a, b := ExtractPlain(f.Body), ExtractPlain(f.Body)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two runs differ", f.Name)
		}
		if a.Version != ExtractorVersion {
			t.Errorf("%s: version = %q", f.Name, a.Version)
		}
	}
}

// TestExtractPlainNeverLosesText feeds many shuffled mixes of the constructs the
// extractor reasons about and checks the one invariant that matters: whatever
// the extractor decides, the blocks in order carry exactly the body's text, and
// the result is valid.
func TestExtractPlainNeverLosesText(t *testing.T) {
	lines := []string{
		"", "", "Hello there", "Thanks,", "Alice", "-- ", "--", "Sent from my iPhone",
		"> quoted", ">> deeper", "  > indented", ">",
		"On Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:",
		"Alice <a@example.org> kirjoitti ma 19.10.2026 klo 9.00:",
		"-----Original Message-----", "________________________________",
		"From: Alice <a@example.org>", "Sent: Monday", "To: Bob", "Subject: Hi",
		"Lähettäjä: Alice", "Lähetetty: maanantai", "Vastaanottaja: Bob", "Aihe: Hei",
		"---------- Forwarded message ---------", "Begin forwarded message:",
		"npm test", "$ make build", "e = mc^2", "\ttabbed text",
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 3000; i++ {
		n := 1 + rng.Intn(14)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = lines[rng.Intn(len(lines))]
		}
		sep := "\n"
		if rng.Intn(4) == 0 {
			sep = "\r\n"
		}
		body := strings.Join(parts, sep)

		got := ExtractPlain(body)
		if err := got.Validate(); err != nil {
			t.Fatalf("invalid result for %q: %v", body, err)
		}
		if NormalizeText(body) == "" {
			if got.Status != StatusUnavailable {
				t.Fatalf("blank body %q: status %q", body, got.Status)
			}
			continue
		}
		var texts []string
		for _, b := range got.Blocks {
			texts = append(texts, b.Content)
		}
		if joined, want := NormalizeText(strings.Join(texts, " ")), NormalizeText(body); joined != want {
			t.Fatalf("text changed for %q\n got: %q\nwant: %q", body, joined, want)
		}
		for _, b := range got.Blocks {
			if b.Source == nil || body[b.Source.Start:b.Source.End] != b.Content {
				t.Fatalf("block %s does not point at the original body for %q", b.ID, body)
			}
		}
	}
}

// Work is linear in the body, so a body just under the limit made of the
// worst-case shapes (many short lines, many blank lines between quotes) is fast.
func TestExtractPlainLargeBodyStaysLinear(t *testing.T) {
	shapes := map[string]string{
		"short lines":      strings.Repeat("a\n", maxPlainBytes/2-1),
		"quote and blanks": strings.Repeat("> q\n\n\n\n", maxPlainBytes/7-1),
		"attributions":     strings.Repeat("On Mon, Oct 19, 2026 at 9:00 AM A wrote:\n\n", maxPlainBytes/45),
		"header lookups":   strings.Repeat("From: x\n", maxPlainBytes/8-1),
	}
	for name, body := range shapes {
		t.Run(name, func(t *testing.T) {
			if len(body) > maxPlainBytes {
				t.Fatalf("test body too large: %d", len(body))
			}
			if err := ExtractPlain(body).Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
		})
	}
}
