package format

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ParseMarkdown converts a markdown string into the chaind internal AST using goldmark.
func ParseMarkdown(input string) []Node {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Linkify),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	src := []byte(input)
	doc := md.Parser().Parse(text.NewReader(src))

	var nodes []Node
	var walk func(n ast.Node, bold bool, italic bool)

	walk = func(n ast.Node, bold bool, italic bool) {
		switch v := n.(type) {
		case *ast.Text:
			nodes = append(nodes, TextNode{Content: string(v.Segment.Value(src)), Bold: bold, Italic: italic})
			return
		case *ast.String:
			nodes = append(nodes, TextNode{Content: string(v.Value), Bold: bold, Italic: italic})
			return
		case *ast.Emphasis:
			if v.Level == 2 {
				bold = true
			} else {
				italic = true
			}
		case *ast.CodeSpan:
			var content string
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				if txt, ok := c.(*ast.Text); ok {
					content += string(txt.Segment.Value(src))
				}
			}
			nodes = append(nodes, CodeNode{Content: content})
			return
		case *ast.Link:
			var label string
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				if txt, ok := c.(*ast.Text); ok {
					label += string(txt.Segment.Value(src))
				}
			}
			nodes = append(nodes, LinkNode{URL: string(v.Destination), Label: label})
			return
		case *ast.AutoLink:
			nodes = append(nodes, LinkNode{URL: string(v.URL(src)), Label: string(v.URL(src))})
			return
		case *ast.FencedCodeBlock:
			var content string
			for i := 0; i < v.Lines().Len(); i++ {
				line := v.Lines().At(i)
				content += string(line.Value(src))
			}
			nodes = append(nodes, BlockNode{Content: content, Language: string(v.Language(src))})
			return
		case *ast.CodeBlock:
			var content string
			for i := 0; i < v.Lines().Len(); i++ {
				line := v.Lines().At(i)
				content += string(line.Value(src))
			}
			nodes = append(nodes, BlockNode{Content: content, Language: ""})
			return
		}

		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, bold, italic)
		}

		// Add newline after block level elements
		switch n.(type) {
		case *ast.Paragraph, *ast.Heading:
			if n.NextSibling() != nil {
				nodes = append(nodes, NewlineNode{})
				nodes = append(nodes, NewlineNode{})
			}
		}
	}

	walk(doc, false, false)
	return nodes
}
