package replycontent

import (
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/net/html"

	"github.com/hkdb/aerion/internal/email"
)

const (
	// maxHTMLBytes bounds the work spent on one body. A larger body is returned
	// whole, sanitized but unanalyzed.
	maxHTMLBytes = 1 << 20
	// maxHTMLDepth is the deepest element nesting analyzed. Deeper documents are
	// returned whole, which also keeps every recursive walk below bounded.
	maxHTMLDepth = 256
)

var (
	// forwardedStartRe matches the text a forwarded message starts with.
	forwardedStartRe = regexp.MustCompile(`(?i)^(-{2,}\s*(forwarded message|edelleenlähetetty viesti|weitergeleitete nachricht)\s*-{2,}|begin forwarded message:?)`)

	sharedSanitizer = sync.OnceValue(email.NewSanitizer)

	// Class and id fragments that mark a client's quoted history, matched case
	// insensitively so prefixed ids such as Outlook web's "x_divRplyFwdMsg" work.
	htmlQuoteMarkers     = []string{"gmail_quote", "yahoo_quoted", "moz-cite-prefix", "outlookmessageheader", "divrplyfwdmsg"}
	htmlSignatureMarkers = []string{"gmail_signature"}

	htmlSkippedElements = map[string]bool{
		"script": true, "style": true, "head": true, "title": true, "meta": true, "link": true,
		"template": true, "noscript": true,
	}
	htmlBlockElements = map[string]bool{
		"address": true, "article": true, "aside": true, "blockquote": true, "body": true, "br": true, "center": true,
		"dd": true, "details": true, "div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true,
		"figure": true, "footer": true, "form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
		"h6": true, "header": true, "hr": true, "li": true, "main": true, "nav": true, "ol": true, "p": true,
		"pre": true, "section": true, "summary": true, "table": true, "tbody": true, "td": true, "tfoot": true,
		"th": true, "thead": true, "tr": true, "ul": true,
	}
	// Elements whose end tag clients always write; a missing or misplaced one is
	// reported as malformed HTML. Elements with optional end tags are not tracked.
	htmlTrackedElements = map[string]bool{
		"div": true, "span": true, "blockquote": true, "table": true, "a": true, "font": true, "pre": true,
		"code": true, "b": true, "i": true, "u": true, "em": true, "strong": true, "ul": true, "ol": true,
	}
)

// htmlKind is the static classification of a DOM subtree.
type htmlKind int

const (
	hBlank htmlKind = iota
	hAuthored
	hQuoted
	hSignature
	hBare  // a blockquote with no client marker; resolved by position
	hMixed // children differ, or the subtree holds the start of a quoted tail
)

type htmlInfo struct {
	kind htmlKind
}

type htmlAtom struct {
	nodes   []*html.Node
	kind    BlockKind
	bare    bool
	text    string
	warning string
}

// ExtractHTML splits an HTML body into authored text, quoted history,
// signatures and content it cannot classify, using client markers in the DOM
// (Gmail, Outlook, Apple Mail, Thunderbird and Yahoo quote containers, Gmail
// signatures, mobile footers). It is conservative in the same way as
// ExtractPlain: unmarked or ambiguous content stays visible as an unknown
// block, and a body with no reliable authored part comes back as one unknown
// block.
//
// Block content is HTML that has been sanitized with the application's email
// policy and with remote images blocked (the original URL is kept in
// data-original-src so the reader can restore it on request). Scripts, event
// handlers, frames and forms never reach a block. Blocks carry no Source: the
// DOM does not keep byte offsets, and the stored body is never modified.
func ExtractHTML(body string) Result {
	if strings.TrimSpace(body) == "" {
		return Result{Version: ExtractorVersion, Status: StatusUnavailable}
	}
	if len(body) > maxHTMLBytes {
		return htmlFallback(sanitizeHTML(body), WarnInputTooLarge)
	}

	var warnings []string
	if looksMalformed(body) {
		warnings = append(warnings, WarnMalformedHTML)
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return htmlFallback(sanitizeHTML(body), append(warnings, WarnMalformedHTML)...)
	}
	root := findBody(doc)
	if root == nil || exceedsDepth(root, maxHTMLDepth) {
		return htmlFallback(sanitizeHTML(body), append(warnings, WarnInputTooLarge)...)
	}

	w := &htmlWalker{info: map[*html.Node]htmlInfo{}}
	w.classify(root)
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		w.visit(c)
	}
	atoms := w.atoms
	if len(atoms) == 0 {
		return Result{Version: ExtractorVersion, Status: StatusUnavailable, Warnings: warnings}
	}

	atoms = markForwarded(atoms)
	atoms = resolveBare(atoms)
	atoms = splitHTMLMobileFooter(atoms)
	atoms = refineHTMLText(atoms)

	var blocks []Block
	hasAuthored, hasQuoted, hasForwarded := false, false, false
	status := StatusExtracted
	for _, atom := range atoms {
		content := sanitizeHTML(renderNodes(atom.nodes))
		if !hasVisibleContent(content) {
			continue
		}
		switch atom.kind {
		case BlockAuthored:
			hasAuthored = true
		case BlockQuoted:
			hasQuoted = true
		case BlockUnknown:
			status = StatusPartial
		}
		if atom.warning == WarnForwardedContent {
			hasForwarded = true
		}
		blocks = append(blocks, Block{ID: blockID(len(blocks) + 1), Kind: atom.kind, Format: PartHTML, Content: content})
		warnings = appendWarning(warnings, atom.warning)
	}
	if len(blocks) == 0 {
		return Result{Version: ExtractorVersion, Status: StatusUnavailable, Warnings: warnings}
	}
	if !hasAuthored {
		var whole strings.Builder
		for c := root.FirstChild; c != nil; c = c.NextSibling {
			whole.WriteString(renderNodes([]*html.Node{c}))
		}
		keep := warnings[:0:0]
		for _, w := range warnings {
			if w == WarnMalformedHTML {
				keep = append(keep, w)
			}
		}
		switch {
		case hasForwarded:
			keep = append(keep, WarnForwardedContent)
		case hasQuoted:
			keep = append(keep, WarnQuoteOnly)
		}
		return htmlFallback(sanitizeHTML(whole.String()), keep...)
	}
	return Result{Version: ExtractorVersion, Status: status, Blocks: blocks, Warnings: warnings}
}

func htmlFallback(content string, warnings ...string) Result {
	result := Result{Version: ExtractorVersion, Status: StatusFallback}
	if !hasVisibleContent(content) {
		result.Status = StatusUnavailable
		return result
	}
	result.Blocks = []Block{{ID: blockID(1), Kind: BlockUnknown, Format: PartHTML, Content: content}}
	for _, w := range warnings {
		result.Warnings = appendWarning(result.Warnings, w)
	}
	return result
}

func sanitizeHTML(s string) string {
	return sharedSanitizer().SanitizeWithRemoteImageBlocking(s)
}

func renderNodes(nodes []*html.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		_ = html.Render(&b, n)
	}
	return b.String()
}

// hasVisibleContent reports whether sanitized HTML shows any text or image.
func hasVisibleContent(content string) bool {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return strings.TrimSpace(content) != ""
	}
	return nodeHasContent(doc)
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBody(c); found != nil {
			return found
		}
	}
	return nil
}

// exceedsDepth reports whether the tree under root nests deeper than limit,
// walking iteratively so a hostile document cannot exhaust the stack.
func exceedsDepth(root *html.Node, limit int) bool {
	type frame struct {
		node  *html.Node
		depth int
	}
	stack := []frame{{root, 0}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.depth > limit {
			return true
		}
		for c := f.node.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, frame{c, f.depth + 1})
		}
	}
	return false
}

// looksMalformed reports unclosed or misplaced end tags among the elements
// clients always close. The parser repairs them, so the tree is usable, but the
// message is worth flagging.
func looksMalformed(body string) bool {
	z := html.NewTokenizer(strings.NewReader(body))
	var stack []string
	for {
		switch z.Next() {
		case html.ErrorToken:
			return z.Err() != io.EOF || len(stack) > 0
		case html.StartTagToken:
			if name, _ := z.TagName(); htmlTrackedElements[string(name)] {
				stack = append(stack, string(name))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if !htmlTrackedElements[string(name)] {
				continue
			}
			index := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == string(name) {
					index = i
					break
				}
			}
			if index < 0 || index != len(stack)-1 {
				return true
			}
			stack = stack[:index]
		}
	}
}

// htmlVisibleText is the text a reader would see under n. Block-level elements
// and line breaks separate words; inline elements do not.
func htmlVisibleText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			if htmlSkippedElements[n.Data] {
				return
			}
			block := htmlBlockElements[n.Data]
			if block {
				b.WriteByte(' ')
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			if block {
				b.WriteByte(' ')
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// nodeHasContent reports whether n shows text or an image.
func nodeHasContent(n *html.Node) bool {
	if n.Type == html.ElementNode && htmlSkippedElements[n.Data] {
		return false
	}
	if n.Type == html.ElementNode && n.Data == "img" {
		return true
	}
	if n.Type == html.TextNode && strings.TrimFunc(n.Data, unicode.IsSpace) != "" {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if nodeHasContent(c) {
			return true
		}
	}
	return false
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.ToLower(a.Val)
		}
	}
	return ""
}

func hasMarker(n *html.Node, markers []string) bool {
	for _, key := range []string{"class", "id"} {
		value := attrValue(n, key)
		for _, marker := range markers {
			if strings.Contains(value, marker) {
				return true
			}
		}
	}
	return false
}

func isQuoteContainer(n *html.Node) bool {
	return hasMarker(n, htmlQuoteMarkers) || (n.Data == "blockquote" && attrValue(n, "type") == "cite")
}

func isSignatureContainer(n *html.Node) bool {
	return hasMarker(n, htmlSignatureMarkers) || attrValue(n, "data-smartmail") == "gmail_signature"
}

// isTailStart reports whether n begins Outlook's reply header, after which the
// rest of the document is quoted history: the header block itself, or the
// horizontal rule that introduces it.
func isTailStart(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if strings.Contains(attrValue(n, "id"), "divrplyfwdmsg") {
		return true
	}
	if n.Data != "hr" {
		return false
	}
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.TextNode && strings.TrimSpace(s.Data) == "" {
			continue
		}
		return s.Type == html.ElementNode && strings.Contains(attrValue(s, "id"), "divrplyfwdmsg")
	}
	return false
}

type htmlWalker struct {
	info  map[*html.Node]htmlInfo
	atoms []htmlAtom
	tail  bool
}

// classify computes the static kind of every node under n: uniform subtrees get
// one kind, anything else is mixed and is walked child by child.
func (w *htmlWalker) classify(n *html.Node) htmlKind {
	kind := w.classifyUncached(n)
	w.info[n] = htmlInfo{kind: kind}
	return kind
}

func (w *htmlWalker) classifyUncached(n *html.Node) htmlKind {
	switch n.Type {
	case html.TextNode:
		if strings.TrimFunc(n.Data, unicode.IsSpace) == "" {
			return hBlank
		}
		return hAuthored
	case html.ElementNode:
	default:
		return hBlank
	}
	if htmlSkippedElements[n.Data] {
		return hBlank
	}
	tail := isTailStart(n)
	agg, seen := hBlank, false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		k := w.classify(c)
		if w.info[c].kind == hMixed && k == hMixed {
			agg, seen = hMixed, true
			continue
		}
		if k == hBlank {
			continue
		}
		switch {
		case !seen:
			agg, seen = k, true
		case agg != k:
			agg = hMixed
		}
	}
	if tail || w.hasTailBelow(n) {
		return hMixed
	}
	if n.Data == "img" {
		return hAuthored
	}
	if !seen {
		return hBlank
	}
	switch {
	case isQuoteContainer(n):
		return hQuoted
	case isSignatureContainer(n):
		return hSignature
	case n.Data == "blockquote" && agg != hMixed:
		return hBare
	}
	return agg
}

func (w *htmlWalker) hasTailBelow(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isTailStart(c) || (w.info[c].kind == hMixed && w.hasTailBelow(c)) {
			return true
		}
	}
	return false
}

func (w *htmlWalker) visit(n *html.Node) {
	if w.info[n].kind == hBlank && !isTailStart(n) {
		return
	}
	if !w.tail && isTailStart(n) {
		w.tail = true
	}
	if w.tail {
		w.emit(n, BlockQuoted, false)
		return
	}
	switch w.info[n].kind {
	case hAuthored:
		w.emit(n, BlockAuthored, false)
	case hQuoted:
		w.emit(n, BlockQuoted, false)
	case hSignature:
		w.emit(n, BlockSignature, false)
	case hBare:
		w.emit(n, BlockQuoted, true)
	case hMixed:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.visit(c)
		}
	}
}

func (w *htmlWalker) emit(n *html.Node, kind BlockKind, bare bool) {
	if !nodeHasContent(n) {
		return
	}
	if last := len(w.atoms) - 1; last >= 0 && w.atoms[last].kind == kind && w.atoms[last].bare == bare {
		w.atoms[last].nodes = append(w.atoms[last].nodes, n)
		return
	}
	w.atoms = append(w.atoms, htmlAtom{nodes: []*html.Node{n}, kind: kind, bare: bare})
}

func nodesText(nodes []*html.Node) string {
	var parts []string
	for _, n := range nodes {
		parts = append(parts, htmlVisibleText(n))
	}
	return NormalizeText(strings.Join(parts, " "))
}

// markForwarded turns everything from the first forwarded-message marker to the
// end into one unknown block: a forward is neither new text nor safe to fold.
func markForwarded(atoms []htmlAtom) []htmlAtom {
	for i := range atoms {
		if atoms[i].kind == BlockSignature {
			continue
		}
		for j, n := range atoms[i].nodes {
			if !forwardedStartRe.MatchString(nodesText([]*html.Node{n})) {
				continue
			}
			head := atoms[:i:i]
			if j > 0 {
				before := atoms[i]
				before.nodes = atoms[i].nodes[:j:j]
				head = append(head, before)
			}
			var rest []*html.Node
			rest = append(rest, atoms[i].nodes[j:]...)
			for _, a := range atoms[i+1:] {
				rest = append(rest, a.nodes...)
			}
			return append(head, htmlAtom{nodes: rest, kind: BlockUnknown, warning: WarnForwardedContent})
		}
	}
	return atoms
}

// resolveBare decides blockquotes that carry no client marker. A preceding
// "... wrote:" line, or no new text after the quote, makes it quoted history;
// otherwise it stays visible as an unknown block.
func resolveBare(atoms []htmlAtom) []htmlAtom {
	for i := range atoms {
		if !atoms[i].bare {
			continue
		}
		atoms[i].bare = false
		if i > 0 && attributionRe.MatchString(nodesText(atoms[i-1].nodes)) {
			continue
		}
		newTextFollows := false
		for _, later := range atoms[i+1:] {
			if later.kind == BlockAuthored {
				newTextFollows = true
				break
			}
		}
		if newTextFollows {
			atoms[i].kind, atoms[i].warning = BlockUnknown, WarnAmbiguousQuoteMarker
		}
	}
	return atoms
}

// splitHTMLMobileFooter splits a trailing "Sent from my iPhone" element off the last
// authored run, but only when real text comes before it.
func splitHTMLMobileFooter(atoms []htmlAtom) []htmlAtom {
	last := -1
	for i, a := range atoms {
		if a.kind == BlockAuthored {
			last = i
		}
	}
	if last < 0 {
		return atoms
	}
	nodes := atoms[last].nodes
	end := len(nodes) - 1
	for end >= 0 && !nodeHasContent(nodes[end]) {
		end--
	}
	if end < 1 || !mobileRe.MatchString(nodesText(nodes[end:end+1])) {
		return atoms
	}
	hasText := false
	for _, n := range nodes[:end] {
		hasText = hasText || nodeHasContent(n)
	}
	if !hasText {
		return atoms
	}
	out := append([]htmlAtom{}, atoms[:last]...)
	out = append(out, htmlAtom{nodes: nodes[:end:end], kind: BlockAuthored})
	out = append(out, htmlAtom{nodes: nodes[end : end+1 : end+1], kind: BlockSignature})
	return append(out, atoms[last+1:]...)
}
