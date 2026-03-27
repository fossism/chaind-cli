package format

import (
	"fmt"
	"strings"
)

type PlainRenderer struct{}

func (p PlainRenderer) Render(nodes []Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch v := n.(type) {
		case TextNode:
			sb.WriteString(v.Content)
		case LinkNode:
			sb.WriteString(fmt.Sprintf("%s (%s)", v.Label, v.URL))
		case MentionNode:
			sb.WriteString(fmt.Sprintf("@%s", v.DisplayName))
		case CodeNode:
			sb.WriteString(v.Content)
		case BlockNode:
			sb.WriteString(fmt.Sprintf("\n%s\n", v.Content))
		case NewlineNode:
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
