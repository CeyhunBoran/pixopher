package ops

// ArgKind is the type of an op argument. The CLI, batch parser, MCP schema
// and guide all derive their parsing and docs from it.
type ArgKind int

// Argument kinds. The text forms are in docs/cli.md (coordinates and
// geometry) and docs/palettes.md (colors).
const (
	KindString ArgKind = iota // free text
	KindInt                   // integer
	KindBool                  // flag without a value
	KindEnum                  // one of ArgSpec.Enum
	KindPoint                 // x,y
	KindSize                  // w,h or wxh
	KindRect                  // x,y,w,h
	KindColor                 // color syntax, resolved against the document by the op
	KindGrid                  // a letter grid block
)

var kindNames = [...]string{"string", "int", "bool", "enum", "point", "size", "rect", "color", "grid"}

// String returns the kind's schema name, such as "point".
func (k ArgKind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// ArgSpec describes one argument. Positional arguments are taken in the order
// they appear in Spec.Args; flags are named.
type ArgSpec struct {
	Name     string
	Kind     ArgKind
	Flag     bool     // --name; otherwise positional
	Short    string   // one-letter flag alias, optional
	Required bool     // only for positionals; flags are always optional
	Repeat   bool     // the last positional may repeat (px pairs, polygon points)
	Default  string   // default in text form, shown in help and omitted from logged lines
	Enum     []string // allowed values for KindEnum
	Help     string
}

// Spec describes an op. It is the single source for the op's CLI command,
// batch syntax, MCP tool, schema entry and guide section.
type Spec struct {
	Name     string // "rect"
	Group    string // "draw"
	Summary  string
	Args     []ArgSpec
	Examples []string
	Mutates  bool // false for observation ops such as show and get
}

// Arg returns the spec of the argument called name.
func (s *Spec) Arg(name string) (ArgSpec, bool) {
	for _, a := range s.Args {
		if a.Name == name {
			return a, true
		}
	}
	return ArgSpec{}, false
}
