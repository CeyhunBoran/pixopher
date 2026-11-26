package script

import (
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// Fields splits one script line into tokens with shell rules: spaces and
// tabs separate tokens; '...' is literal; "..." is literal apart from
// backslash before \, ", $ and `; a backslash outside quotes takes the next
// character as is. A `#` that starts a token and is followed by a space, a
// tab or the end of the line starts a comment, which runs to the end of the
// line. Any other `#` is part of a token, so hex colors (#ff8800) need no
// quoting. It reads what ops.Quote writes. An unterminated quote or a
// trailing backslash is a usage error.
func Fields(line string) ([]string, error) {
	out, _, err := fields(line)
	return out, err
}

// fields is Fields that also reports, for each token, whether it was
// written bare: without quotes or backslashes.
func fields(line string) ([]string, []bool, error) {
	var (
		out  []string
		bare []bool
		tok  strings.Builder
		in   bool // inside a token, which may be empty ('')
		lit  bool // the token has quotes or a backslash
	)
	end := func() {
		if in {
			out = append(out, tok.String())
			bare = append(bare, !lit)
			tok.Reset()
			in, lit = false, false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			end()
		case c == '#' && !in && (i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t'):
			return out, bare, nil
		case c == '\'':
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, nil, errs.New(errs.Usage, "unterminated ' quote").WithHint("close the quote on the same line")
			}
			tok.WriteString(line[i+1 : i+1+j])
			i += j + 1
			in, lit = true, true
		case c == '"':
			j := i + 1
			for ; j < len(line) && line[j] != '"'; j++ {
				if line[j] == '\\' && j+1 < len(line) && strings.IndexByte("\\\"$`", line[j+1]) >= 0 {
					j++
				}
				tok.WriteByte(line[j])
			}
			if j == len(line) {
				return nil, nil, errs.New(errs.Usage, `unterminated " quote`).WithHint("close the quote on the same line")
			}
			i = j
			in, lit = true, true
		case c == '\\':
			if i+1 == len(line) {
				return nil, nil, errs.New(errs.Usage, "trailing backslash").WithHint("a line can't continue on the next one")
			}
			i++
			tok.WriteByte(line[i])
			in, lit = true, true
		default:
			tok.WriteByte(c)
			in = true
		}
	}
	end()
	return out, bare, nil
}
