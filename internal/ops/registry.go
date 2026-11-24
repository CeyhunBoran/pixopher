package ops

import (
	"fmt"
	"slices"
	"strings"
)

// Registry holds ops by name.
type Registry struct {
	ops map[string]Op
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{ops: map[string]Op{}} }

// Default is the registry of built-in ops. The CLI, batch scripts, MCP,
// schema and guide are generated from it.
var Default = NewRegistry()

// Register adds op. It panics on an invalid spec or a duplicate name, since
// both are programming errors caught by any test that loads the registry.
func (r *Registry) Register(op Op) {
	s := op.Spec()
	if err := s.check(); err != nil {
		panic(err)
	}
	if _, dup := r.ops[s.Name]; dup {
		panic(fmt.Sprintf("ops: duplicate op %q", s.Name))
	}
	r.ops[s.Name] = op
}

// Lookup returns the op called name.
func (r *Registry) Lookup(name string) (Op, bool) {
	op, ok := r.ops[name]
	return op, ok
}

// All returns every op, sorted by group and then name.
func (r *Registry) All() []Op {
	all := make([]Op, 0, len(r.ops))
	for _, op := range r.ops {
		all = append(all, op)
	}
	slices.SortFunc(all, func(a, b Op) int {
		sa, sb := a.Spec(), b.Spec()
		if c := strings.Compare(sa.Group, sb.Group); c != 0 {
			return c
		}
		return strings.Compare(sa.Name, sb.Name)
	})
	return all
}

// check reports spec mistakes that would break CLI or schema generation.
func (s *Spec) check() error {
	if s.Name == "" || s.Group == "" || s.Summary == "" {
		return fmt.Errorf("ops: spec %q needs a name, group and summary", s.Name)
	}
	seen := map[string]bool{}
	optional := false
	for i, a := range s.Args {
		switch {
		case a.Name == "":
			return fmt.Errorf("ops: %s: argument %d has no name", s.Name, i)
		case seen[a.Name]:
			return fmt.Errorf("ops: %s: duplicate argument %q", s.Name, a.Name)
		case a.Kind < KindString || a.Kind > KindGrid:
			return fmt.Errorf("ops: %s: argument %q has an unknown kind", s.Name, a.Name)
		case a.Kind == KindEnum && len(a.Enum) == 0:
			return fmt.Errorf("ops: %s: enum argument %q has no values", s.Name, a.Name)
		case a.Flag && (a.Required || a.Repeat):
			return fmt.Errorf("ops: %s: flag %q cannot be required or repeated", s.Name, a.Name)
		case !a.Flag && a.Short != "":
			return fmt.Errorf("ops: %s: positional %q cannot have a short flag", s.Name, a.Name)
		case !a.Flag && a.Kind == KindBool:
			return fmt.Errorf("ops: %s: bool argument %q must be a flag", s.Name, a.Name)
		case len(a.Short) > 1 || a.Short != "" && isDigit(a.Short[0]):
			return fmt.Errorf("ops: %s: short flag %q must be one non-digit character", s.Name, a.Short)
		case a.Default != "" && a.Required:
			return fmt.Errorf("ops: %s: required argument %q cannot have a default", s.Name, a.Name)
		}
		if a.Default != "" {
			toks := strings.Fields(a.Default)
			if a.Flag {
				toks = []string{a.Default}
			}
			if _, n, err := parseValue(a, toks); err != nil || n != len(toks) {
				return fmt.Errorf("ops: %s: bad default %q for %q", s.Name, a.Default, a.Name)
			}
		}
		seen[a.Name] = true
		if a.Flag {
			continue
		}
		if a.Required && optional {
			return fmt.Errorf("ops: %s: required positional %q follows an optional one", s.Name, a.Name)
		}
		if !a.Required {
			optional = true
		}
		if a.Repeat && i != lastPositional(s.Args) {
			return fmt.Errorf("ops: %s: only the last positional may repeat, not %q", s.Name, a.Name)
		}
	}
	return nil
}

func lastPositional(args []ArgSpec) int {
	last := -1
	for i, a := range args {
		if !a.Flag {
			last = i
		}
	}
	return last
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
