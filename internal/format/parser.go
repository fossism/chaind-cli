package format

import (
	"strings"
)

// ParseMarkdown is a highly simplified parser for the demo.
// In a full implementation, this uses github.com/yuin/goldmark to trace the ast.Node tree.
func ParseMarkdown(input string) []Node {
	var nodes []Node
	
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		// Very basic block code detection
		if strings.HasPrefix(line, "```") {
			lang := strings.TrimPrefix(line, "```")
			content := ""
			// We just consume the rest for demo simplicity here
			// A real parser walks goldmark
			nodes = append(nodes, BlockNode{Content: content, Language: lang})
			break
		}
		
		// Fallback to text node for line
		nodes = append(nodes, TextNode{Content: line})
		
		if i < len(lines)-1 {
			nodes = append(nodes, NewlineNode{})
		}
	}
	
	return nodes
}
