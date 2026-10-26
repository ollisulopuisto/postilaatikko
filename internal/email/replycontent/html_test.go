package replycontent

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// textOfHTML is the visible text of a block's HTML, whitespace collapsed.
func textOfHTML(t *testing.T, content string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse block: %v", err)
	}
	return NormalizeText(htmlVisibleText(doc))
}

func TestExtractHTMLMatchesFixtures(t *testing.T) {
	for _, f := range loadFixtures(t) {
		if f.Format != "html" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			got := ExtractHTML(f.Body)
			if err := got.Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
			if got.Status != f.Expect.Status {
				t.Errorf("status = %q, want %q (blocks %v)", got.Status, f.Expect.Status, kindsOf(got))
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
				if b.Format != PartHTML {
					t.Errorf("block %d format = %q, want html", i+1, b.Format)
				}
				if text := textOfHTML(t, b.Content); text != want.Text {
					t.Errorf("block %d text = %q, want %q", i+1, text, want.Text)
				}
			}
		})
	}
}

func TestExtractHTMLEdgeCases(t *testing.T) {
	const attribution = `<div class="gmail_attr">On Mon, Oct 19, 2026 at 9:00 AM Alice wrote:</div>`
	tests := []struct {
		name     string
		body     string
		status   Status
		kinds    []BlockKind
		warnings []string
	}{
		{"empty", "", StatusUnavailable, nil, nil},
		{"only markup", "<div><br></div><p>&nbsp;</p>", StatusUnavailable, nil, nil},
		{"new message only", "<div>Hello</div><div>Bye</div>", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"bare blockquote at the end is quoted history",
			"<div>Thanks.</div><blockquote>earlier</blockquote>", StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"bare blockquote followed by new text stays visible",
			"<blockquote>earlier question</blockquote><div>My answer.</div>",
			StatusPartial, []BlockKind{BlockUnknown, BlockAuthored}, []string{WarnAmbiguousQuoteMarker}},
		{"bare blockquote after a wrote line is quoted",
			"<div>Yes.</div><div>On Mon, Alice wrote:</div><blockquote>earlier</blockquote><div>More.</div>",
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted, BlockAuthored}, nil},
		{"yahoo quoted container",
			`<div>Yes.</div><div id="yahoo_quoted_123"><div>On Monday, Alice wrote:</div><div>earlier</div></div>`,
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"outlook reply header without a rule",
			`<div>Fine.</div><div id="divRplyFwdMsg"><b>From:</b> Alice<br><b>Subject:</b> Hi</div><div>old body</div><div>more old</div>`,
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"outlook web prefixed ids",
			`<div>Fine.</div><hr><div id="x_divRplyFwdMsg"><b>From:</b> Alice</div><div>old body</div>`,
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"quote nested inside authored wrappers",
			`<div><div>Yes.</div><div class="gmail_quote">` + attribution + `<blockquote class="gmail_quote">old</blockquote></div></div>`,
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
		{"gmail forwarded message is not collapsed",
			`<div>FYI</div><div class="gmail_quote"><div class="gmail_attr">---------- Forwarded message ---------<br>From: A</div><div>body</div></div>`,
			StatusPartial, []BlockKind{BlockAuthored, BlockUnknown}, []string{WarnForwardedContent}},
		{"forwarded only is a fallback",
			`<div class="gmail_quote"><div class="gmail_attr">---------- Forwarded message ---------</div><div>body</div></div>`,
			StatusFallback, []BlockKind{BlockUnknown}, []string{WarnForwardedContent}},
		{"mobile footer", "<div>On my way.</div><div>Sent from my iPhone</div>", StatusExtracted, []BlockKind{BlockAuthored, BlockSignature}, nil},
		{"mobile footer alone stays authored", "<div>Sent from my iPhone</div>", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"image only message is content", `<img src="cid:logo@example.org">`, StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"unclosed div is reported", "<div>Hi<div>there", StatusExtracted, []BlockKind{BlockAuthored}, []string{WarnMalformedHTML}},
		{"stray end tag is reported", "<div>Hi</div></div>", StatusExtracted, []BlockKind{BlockAuthored}, []string{WarnMalformedHTML}},
		{"optional end tags are not reported", "<p>one<p>two<ul><li>a<li>b</ul>", StatusExtracted, []BlockKind{BlockAuthored}, nil},
		{"a quote inside a list item splits the list",
			`<ul><li>one</li><li class="gmail_quote">quoted</li></ul>`,
			StatusExtracted, []BlockKind{BlockAuthored, BlockQuoted}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHTML(tt.body)
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

// Meaningful structure survives in the block it belongs to.
func TestExtractHTMLPreservesStructure(t *testing.T) {
	body := `<p>Plan:</p><ul><li>one</li><li>two</li></ul>` +
		`<table><tr><th>A</th><td>1</td></tr></table>` +
		`<pre><code>if a &gt; b {\n  run()\n}</code></pre>` +
		`<img src="cid:diagram@example.org" alt="diagram">` +
		`<div class="gmail_quote"><blockquote class="gmail_quote"><img src="cid:old@example.org"><p>old</p></blockquote></div>`
	got := ExtractHTML(body)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(kindsOf(got)) != "[authored quoted]" {
		t.Fatalf("kinds = %v", kindsOf(got))
	}
	authored, quoted := got.Blocks[0].Content, got.Blocks[1].Content
	for _, want := range []string{"<ul>", "<li>one</li>", "<table>", "<th>A</th>", "<pre>", "<code>", "cid:diagram@example.org", "&gt; b"} {
		if !strings.Contains(authored, want) {
			t.Errorf("authored block lost %q:\n%s", want, authored)
		}
	}
	if strings.Contains(authored, "cid:old@example.org") {
		t.Error("the cid image of the quoted part leaked into the authored block")
	}
	if !strings.Contains(quoted, "cid:old@example.org") {
		t.Errorf("quoted block lost its cid image:\n%s", quoted)
	}
}

// Active content never reaches a block, and remote images stay blocked, in
// every kind of block and in the fallback.
// remoteSrcRe matches a live src attribute, not data-original-src, which only
// remembers the blocked URL for the reader.
var remoteSrcRe = regexp.MustCompile(`\ssrc="https?://`)

func TestExtractHTMLNeutralizesActiveContent(t *testing.T) {
	const attack = `<script>stealCookies()</script>` +
		`<img src="https://tracker.example/pixel.gif" onerror="alert(1)">` +
		`<a href="javascript:alert(2)" onclick="alert(3)">click</a>` +
		`<iframe src="https://evil.example/"></iframe><object data="x"></object><embed src="x">` +
		`<form action="https://evil.example/"><input name="p"></form>` +
		`<div style="background:url(https://tracker.example/bg.png)">styled</div>` +
		`<style>@import url(https://tracker.example/x.css);</style>`
	bodies := map[string]string{
		"authored":      `<div>Hello</div>` + attack,
		"quoted":        `<div>Hello</div><div class="gmail_quote"><blockquote class="gmail_quote">` + attack + `</blockquote></div>`,
		"fallback":      `<div class="gmail_quote"><blockquote class="gmail_quote">` + attack + `</blockquote></div>`,
		"outlook tail":  `<div>Hello</div><hr><div id="divRplyFwdMsg">From: A</div>` + attack,
		"signature":     `<div>Hello</div><div class="gmail_signature">` + attack + `</div>`,
		"top-level run": attack,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			got := ExtractHTML(body)
			if err := got.Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
			for _, b := range got.Blocks {
				lower := strings.ToLower(b.Content)
				for _, bad := range []string{"<script", "stealcookies", "onerror", "onclick", "javascript:", "<iframe", "<object", "<embed", "<form", "<input", "@import"} {
					if strings.Contains(lower, bad) {
						t.Errorf("block %s (%s) still contains %q:\n%s", b.ID, b.Kind, bad, b.Content)
					}
				}
				if remoteSrcRe.MatchString(lower) || strings.Contains(lower, "url(https://") {
					t.Errorf("block %s loads a remote resource:\n%s", b.ID, b.Content)
				}
			}
		})
	}
	t.Run("blocked images can be restored by the reader", func(t *testing.T) {
		got := ExtractHTML(`<div>Hi <img src="https://tracker.example/pixel.gif"></div>`)
		if len(got.Blocks) != 1 || !strings.Contains(got.Blocks[0].Content, `data-original-src="https://tracker.example/pixel.gif"`) {
			t.Fatalf("remote image not blocked with its original kept: %+v", got.Blocks)
		}
	})
}

func TestExtractHTMLIsBounded(t *testing.T) {
	cases := map[string]string{
		"oversized": "<div>Tuesday works.</div>" + strings.Repeat("<div>x</div>", maxHTMLBytes/12+10),
		"too deep":  strings.Repeat("<div>", maxHTMLDepth+50) + "deep text" + strings.Repeat("</div>", maxHTMLDepth+50),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got := ExtractHTML(body)
			if err := got.Validate(); err != nil {
				t.Fatalf("invalid result: %v", err)
			}
			if got.Status != StatusFallback || len(got.Blocks) != 1 || got.Blocks[0].Kind != BlockUnknown {
				t.Fatalf("status %q kinds %v, want a single unknown block", got.Status, kindsOf(got))
			}
			if len(got.Warnings) != 1 || got.Warnings[0] != WarnInputTooLarge {
				t.Errorf("warnings = %v, want [%s]", got.Warnings, WarnInputTooLarge)
			}
		})
	}
	t.Run("a wide document just under the limit", func(t *testing.T) {
		body := strings.Repeat("<div>x</div>", maxHTMLBytes/12-5)
		if err := ExtractHTML(body).Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestExtractHTMLIsDeterministic(t *testing.T) {
	for _, f := range loadFixtures(t) {
		if f.Format != "html" {
			continue
		}
		if a, b := ExtractHTML(f.Body), ExtractHTML(f.Body); !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two runs differ", f.Name)
		}
	}
}

// TestExtractHTMLNeverLosesText mixes the constructs the extractor reasons about
// in random order and checks that, whatever it decides, the blocks in order
// carry exactly the visible characters of the body and the result is valid.
func TestExtractHTMLNeverLosesText(t *testing.T) {
	snippets := []string{
		"<div>Hello there</div>", "<br>", "plain text", "<hr>", "<p>", "</div>", "<div>", "</blockquote>",
		`<div class="gmail_quote"><div class="gmail_attr">On Mon, A wrote:</div><blockquote class="gmail_quote"><div>Old words</div></blockquote></div>`,
		"<blockquote>bare quote</blockquote>", `<blockquote type="cite">cited</blockquote>`,
		`<div id="divRplyFwdMsg"><b>From:</b> A<br><b>Sent:</b> Monday</div>`,
		`<div class="gmail_signature">Sig line</div>`, "<div>Sent from my iPhone</div>",
		"<div>---------- Forwarded message ---------</div>", `<div class="gmail_quote"><div>---------- Forwarded message ---------</div><div>fwd body</div></div>`,
		"<pre>code &gt; here</pre>", "<table><tr><td>cell</td><td>two</td></tr></table>", "<ul><li>item</li></ul>",
		`<img src="cid:x">`, `<img src="https://t.example/x.gif" alt="pixel">`, "<script>evil()</script>", "<style>p{}</style>",
		"<span>inline</span>", "<b>bold", "&nbsp;", "<td>stray cell</td>", "<li>stray item",
	}
	rng := rand.New(rand.NewSource(11))
	strip := func(s string) string { return strings.Join(strings.Fields(s), "") }
	for i := 0; i < 2500; i++ {
		n := 1 + rng.Intn(10)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = snippets[rng.Intn(len(snippets))]
		}
		body := strings.Join(parts, "")

		got := ExtractHTML(body)
		if err := got.Validate(); err != nil {
			t.Fatalf("invalid result for %q: %v", body, err)
		}
		doc, err := html.Parse(strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		want := strip(htmlVisibleText(doc))
		if want == "" {
			continue
		}
		var gotText strings.Builder
		for _, b := range got.Blocks {
			gotText.WriteString(textOfHTML(t, b.Content))
		}
		if strip(gotText.String()) != want {
			t.Fatalf("text changed for %q\n got: %q\nwant: %q", body, strip(gotText.String()), want)
		}
	}
}
