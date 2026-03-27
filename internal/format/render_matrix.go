package format

import (
	"fmt"
	"strings"
)

type MatrixRenderer struct{}

func (m MatrixRenderer) Render(nodes []Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch v := n.(type) {
		case TextNode:
			text := v.Content
			if v.Bold {
				text = "**" + text + "**"
			}
			if v.Italic {
				text = "*" + text + "*"
			}
			sb.WriteString(text)
		case LinkNode:
			sb.WriteString(fmt.Sprintf("[%s](%s)", v.Label, v.URL))
		case MentionNode:
			sb.WriteString(fmt.Sprintf("[%s](https://matrix.to/#/%s)", v.DisplayName, v.UserID))
		case CodeNode:
			sb.WriteString(fmt.Sprintf("`%s`", v.Content))
		case BlockNode:
			sb.WriteString(fmt.Sprintf("```%s\n%s\n```", v.Language, v.Content))
		case NewlineNode:
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
