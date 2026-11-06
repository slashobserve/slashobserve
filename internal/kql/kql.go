package kql

type FieldKind string

const (
	Keyword FieldKind = "keyword"
	Text    FieldKind = "text"
	Number  FieldKind = "number"
	Date    FieldKind = "date"
	Boolean FieldKind = "boolean"
	IP      FieldKind = "ip"
	Nested  FieldKind = "nested"
	Object  FieldKind = "object"
	Other   FieldKind = "other"
)

type Fields map[string]FieldKind

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

type Diagnostic struct {
	Severity string
	Message  string
	Start    int
	End      int
}

type Compiled struct {
	Query       map[string]any
	Diagnostics []Diagnostic
}

type Kind string

const (
	And    Kind = "and"
	Or     Kind = "or"
	Not    Kind = "not"
	Match  Kind = "match"
	Range  Kind = "range"
	Exists Kind = "exists"
	Free   Kind = "free"
	Group  Kind = "group"
)

type Node struct {
	Kind     Kind
	Field    string
	Op       string
	Value    string
	Wildcard bool
	Quoted   bool
	Children []*Node
	Start    int
	End      int

	fieldStart, fieldEnd int
	segments             []segment
}
