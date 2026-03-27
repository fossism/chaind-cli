package format

import (
	"fmt"
	"strings"
)

type TelegramRenderer struct{}

func (t TelegramRenderer) Render(nodes []Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch v := n.(type) {
		case TextNode:
			text := v.Content
			if v.Bold {
				text = "<b>" + text + "</b>"
			}
			if v.Italic {
				text = "<i>" + text + "</i>"
			}
			sb.WriteString(text)
		case LinkNode:
			sb.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a>", v.URL, v.Label))
		case MentionNode:
			sb.WriteString(fmt.Sprintf("<a href=\"tg://user?id=%s\">%s</a>", v.UserID, v.DisplayName))
		case CodeNode:
			sb.WriteString(fmt.Sprintf("<code>%s</code>", v.Content))
		case BlockNode:
			sb.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", v.Language, v.Content))
		case NewlineNode:
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
