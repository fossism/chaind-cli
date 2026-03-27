package format

// Node is the base interface for all formatting AST nodes
type Node interface {
	NodeType() string
}

type TextNode struct {
	Content string
	Bold    bool
	Italic  bool
}

func (n TextNode) NodeType() string { return "text" }

type LinkNode struct {
	URL   string
	Label string
}

func (n LinkNode) NodeType() string { return "link" }

type MentionNode struct {
	UserID      string
	DisplayName string
}

func (n MentionNode) NodeType() string { return "mention" }

type CodeNode struct {
	Content string
}

func (n CodeNode) NodeType() string { return "code" }

type BlockNode struct {
	Content  string
	Language string
}

func (n BlockNode) NodeType() string { return "block" }

type NewlineNode struct{}

func (n NewlineNode) NodeType() string { return "newline" }

// Renderer is the interface that each platform implements to convert AST to its format
type Renderer interface {
	Render(nodes []Node) string
}
