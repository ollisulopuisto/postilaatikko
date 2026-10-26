package replycontent

import (
	"strings"

	"golang.org/x/net/html"
)

type textRange struct{ start, end int }

// refineHTMLText applies plain-text reply/signature boundaries to unmarked HTML
// while cloning the original DOM ranges. Formatting, links and images survive;
// extracted HTML still goes through the same sanitizer as all other blocks.
func refineHTMLText(atoms []htmlAtom) []htmlAtom {
	var out []htmlAtom
	for _, atom := range atoms {
		if atom.kind != BlockAuthored {
			out = append(out, atom)
			continue
		}
		var text strings.Builder
		ranges := map[*html.Node]textRange{}
		var index func(*html.Node)
		index = func(n *html.Node) {
			start := text.Len()
			if n.Type == html.TextNode {
				text.WriteString(n.Data)
			} else if n.Type == html.ElementNode {
				if htmlSkippedElements[n.Data] {
					ranges[n] = textRange{start, start}
					return
				}
				if htmlBlockElements[n.Data] {
					text.WriteByte('\n')
				}
				if n.Data == "img" {
					text.WriteString("\uFFFC")
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					index(c)
				}
				if htmlBlockElements[n.Data] {
					text.WriteByte('\n')
				}
			}
			ranges[n] = textRange{start, text.Len()}
		}
		for _, n := range atom.nodes {
			index(n)
		}
		split := ExtractPlain(text.String())
		if split.Status != StatusExtracted || len(split.Blocks) < 2 || len(split.Blocks) > 32 {
			out = append(out, atom)
			continue
		}
		valid := true
		for _, b := range split.Blocks {
			if b.Source == nil {
				valid = false
			}
		}
		if !valid {
			out = append(out, atom)
			continue
		}
		for _, b := range split.Blocks {
			nodes := []*html.Node{}
			for _, n := range atom.nodes {
				if clone := cloneHTMLRange(n, ranges, b.Source.Start, b.Source.End); clone != nil {
					nodes = append(nodes, clone)
				}
			}
			out = append(out, htmlAtom{nodes: nodes, kind: b.Kind, text: b.Content})
		}
	}
	return out
}

func cloneHTMLRange(n *html.Node, ranges map[*html.Node]textRange, start, end int) *html.Node {
	r := ranges[n]
	if r.end <= start || r.start >= end {
		return nil
	}
	clone := &html.Node{Type: n.Type, DataAtom: n.DataAtom, Data: n.Data, Namespace: n.Namespace, Attr: append([]html.Attribute(nil), n.Attr...)}
	if n.Type == html.TextNode {
		from, to := max(start, r.start)-r.start, min(end, r.end)-r.start
		clone.Data = n.Data[from:to]
		return clone
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := cloneHTMLRange(c, ranges, start, end); child != nil {
			clone.AppendChild(child)
		}
	}
	return clone
}
