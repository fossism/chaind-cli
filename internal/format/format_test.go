package format

import (
	"testing"
)

func TestMatrixRenderer(t *testing.T) {
	// Our simple AST parser output to Render
	input := []Node{
		TextNode{Content: "Hello ", Bold: true},
		MentionNode{DisplayName: "Alice", UserID: "@alice:example.com"},
	}

	mr := MatrixRenderer{}
	out := mr.Render(input)
	
	expected := "**Hello **[Alice](https://matrix.to/#/@alice:example.com)"
	if out != expected {
		t.Errorf("Expected %q but got %q", expected, out)
	}
}

func TestTelegramRenderer(t *testing.T) {
	input := []Node{
		TextNode{Content: "Hello ", Bold: true},
		MentionNode{DisplayName: "Alice", UserID: "12345"},
	}

	tr := TelegramRenderer{}
	out := tr.Render(input)
	
	expected := "<b>Hello </b><a href=\"tg://user?id=12345\">Alice</a>"
	if out != expected {
		t.Errorf("Expected %q but got %q", expected, out)
	}
}

func TestPlainRenderer(t *testing.T) {
	input := []Node{
		TextNode{Content: "Hello ", Bold: true},
		MentionNode{DisplayName: "Alice", UserID: "12345"},
	}

	pr := PlainRenderer{}
	out := pr.Render(input)
	
	expected := "Hello @Alice"
	if out != expected {
		t.Errorf("Expected %q but got %q", expected, out)
	}
}
