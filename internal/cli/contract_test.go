package cli

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// The output contract from docs/cli.md#output-done. session.run checks it
// on every command of every transcript, so a golden file can't quietly
// record output that breaks it.

// wantJSON works out from the arguments and the environment, the way
// docs/cli.md describes it, whether the output must be JSON. It reports
// false for help and completion, which print text either way (D59).
func wantJSON(args []string, env map[string]string) (on, checked bool) {
	if len(args) == 0 || args[0] == "help" || args[0] == "completion" {
		return false, false
	}
	on, _ = strconv.ParseBool(env["PIXOPHER_JSON"])
	for _, a := range args {
		switch {
		case a == "--":
			return on, true
		case a == "-h" || a == "--help":
			return false, false
		case a == "--json":
			on = true
		case strings.HasPrefix(a, "--json="):
			if b, err := strconv.ParseBool(a[len("--json="):]); err == nil {
				on = b
			}
		}
	}
	return on, true
}

var humanErr = regexp.MustCompile(`^error\[([a-z_]+)\]: .+\n(hint: .+\n)?$`)

// checkContract fails t when one command's output breaks the contract.
// mutating says whether the command changes a document (an op with
// Mutates, undo or redo), whose success output has a fixed set of keys.
func checkContract(t *testing.T, line string, args []string, env map[string]string, mutating bool, code int, stdout, stderr string) {
	t.Helper()
	fail := func(format string, a ...any) {
		t.Helper()
		t.Errorf("pxo %s: "+format, append([]any{line}, a...)...)
	}
	on, checked := wantJSON(args, env)
	if !checked {
		return
	}
	if !on {
		if code == 0 {
			return
		}
		m := humanErr.FindStringSubmatch(stderr)
		switch {
		case stdout != "":
			fail("an error wrote to stdout: %q", stdout)
		case m == nil:
			fail("stderr %q is not error[<code>]: ... with an optional hint line", stderr)
		case !slices.Contains(errs.All(), errs.Code(m[1])):
			fail("unknown error code %q", m[1])
		case errs.ExitCode(errs.Code(m[1])) != code:
			fail("exit %d, but %s exits %d", code, m[1], errs.ExitCode(errs.Code(m[1])))
		}
		return
	}

	if stderr != "" {
		fail("--json wrote to stderr: %q", stderr)
	}
	if strings.Count(stdout, "\n") != 1 || !strings.HasSuffix(stdout, "\n") {
		fail("--json stdout is not exactly one line: %q", stdout)
		return
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(stdout)))
	dec.UseNumber()
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		fail("--json stdout is not a JSON object: %v", err)
		return
	}
	var ok bool
	if err := json.Unmarshal(obj["ok"], &ok); err != nil {
		fail(`no boolean "ok": %s`, stdout)
		return
	}
	if ok != (code == 0) {
		fail(`"ok" is %v but the exit code is %d`, ok, code)
	}
	if !ok {
		checkEnvelope(fail, obj, code)
		return
	}
	if !mutating {
		return
	}
	var changed int
	if err := json.Unmarshal(obj["changed"], &changed); err != nil {
		fail(`no integer "changed": %s`, stdout)
	}
	for _, k := range []string{"rev", "warnings"} {
		if _, has := obj[k]; !has {
			fail("no %q: %s", k, stdout)
		}
	}
	if _, has := obj["bbox"]; has != (changed > 0) {
		fail(`"bbox" must be there exactly when "changed" > 0: %s`, stdout)
	}
	dry := slices.Contains(args, "--dry-run")
	if raw, has := obj["dry_run"]; has != dry || (has && string(raw) != "true") {
		fail(`"dry_run": true must be there exactly with --dry-run: %s`, stdout)
	}
	for k := range obj {
		if !slices.Contains([]string{"ok", "rev", "changed", "bbox", "warnings", "dry_run", "undone", "redone"}, k) {
			fail("unexpected key %q: %s", k, stdout)
		}
	}
}

// checkEnvelope checks the error envelope {"ok": false, "error": {...}}.
func checkEnvelope(fail func(string, ...any), obj map[string]json.RawMessage, code int) {
	if len(obj) != 2 {
		fail(`the error envelope has keys other than "ok" and "error"`)
	}
	var e map[string]json.RawMessage
	if err := json.Unmarshal(obj["error"], &e); err != nil {
		fail(`"error" is not an object: %v`, err)
		return
	}
	for k := range e {
		if !slices.Contains([]string{"code", "message", "hint", "arg", "line", "cause", "details"}, k) {
			fail("unexpected error key %q", k)
		}
	}
	// A script_error, and only a script_error, names its line and the code
	// it wraps.
	_, line := e["line"]
	_, cause := e["cause"]
	if script := string(e["code"]) == `"script_error"`; line != script || cause != script {
		fail("line and cause must be given exactly for a script_error")
	}
	for _, k := range []string{"hint", "arg"} {
		if string(e[k]) == `""` {
			fail("an empty %q is not omitted", k)
		}
	}
	var c errs.Code
	var msg string
	_ = json.Unmarshal(e["code"], &c)
	_ = json.Unmarshal(e["message"], &msg)
	switch {
	case !slices.Contains(errs.All(), c):
		fail("unknown error code %q", c)
	case msg == "":
		fail("empty error message")
	case errs.ExitCode(c) != code:
		fail("exit %d, but %s exits %d", code, c, errs.ExitCode(c))
	}
}

func TestWantJSON(t *testing.T) {
	for _, tt := range []struct {
		args    string
		env     string
		on, chk bool
	}{
		{"px 1,1 ink", "", false, true},
		{"px 1,1 ink --json", "", true, true},
		{"--json -f a.pxo px 1,1 ink", "", true, true},
		{"px 1,1 ink", "1", true, true},
		{"px 1,1 ink --json=false", "true", false, true},
		{"px 1,1 ink -- --json", "", false, true},
		{"px --help --json", "", false, false},
		{"completion bash --json", "", false, false},
	} {
		on, chk := wantJSON(strings.Fields(tt.args), map[string]string{"PIXOPHER_JSON": tt.env})
		if on != tt.on || chk != tt.chk {
			t.Errorf("wantJSON(%q, env %q) = %v, %v; want %v, %v", tt.args, tt.env, on, chk, tt.on, tt.chk)
		}
	}
}
