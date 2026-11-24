# Decision log

Short records of decisions and why they were made. Add new entries at the
bottom. Don't edit old entries. To change a decision, add a new entry that
supersedes it.

Format: **D<n>: title** (date), then the decision, why, and consequences.

---

**D1: Go, pure Go, no CGO** (2026-11-23)
A single static binary that cross-compiles everywhere and works in
containers and over SSH. This rules out native GUI toolkits and C image
libraries, so encoders that the stdlib lacks (WebP) must be pure Go.

**D2: Hybrid state, with the file as the source of truth** (2026-11-23)
Every command works directly on the `.pxo` file (lock, load, apply, atomic
save, log). An optional `serve` daemon keeps documents hot, pushes live
events, and writes through to disk.
*Why*: in-memory-only daemons lose work and make lifecycle management a
burden for agents, while file-only mode can't stream individual strokes.
Both modes must produce identical results.

**D3: Readable text format, TOML with letter-grid cels** (2026-11-23)
Git diffs show pixel changes, and humans and agents can read and edit files
directly. RLE handles large cels. History is a separate JSONL sidecar, so
the main file stays clean.

**D4: Palette-first color model, raw RGBA allowed** (2026-11-23)
Named entries and ramps enable shading, selective outline, recolor and
variants, and keep grids readable. A raw hex becomes an auto entry, so grids
only contain keys.

**D5: One op registry drives the CLI, batch, MCP, schema and guide** (2026-11-23)
This prevents drift between interfaces and keeps docs generated from code.

**D6: Layers and frames in the model from day one** (2026-11-23)
They shape the file format and the compositing pipeline, so retrofitting them
would be expensive. M1 exposes only one layer and one frame, but stores the
full structure.

**D7: Live viewing in the terminal and the browser, no native window** (2026-11-23)
`watch` (kitty/sixel/halfblock/ascii) works anywhere a terminal does. The web
viewer adds a timeline, playback and recording. A native window would need
GUI dependencies (conflicts with D1).

**D8: Support JPEG, but always warn and flatten** (2026-11-23)
JPEG is useful for previews and sharing, but it blurs pixel art. WebP is
lossless by default.

**D9: Maximum canvas size 1024x1024** (2026-11-23)
This covers scenes and backgrounds. The viewer needs viewport panning, and
grids and RLE have to scale.

**D10: Names: `pixopher` binary, `pxo` alias, `.pxo` extension** (2026-11-23)

**D11: English docs, GPL-3.0 license** (2026-11-23)
The repo is public. The license is the one already in the repository.

**D12: Agent-first feedback features are core, not extras** (2026-11-23)
`look`, `lint`, regions/anchors, and narrated replay/timelapse are scheduled
in early milestones (M2–M7). They are the main reason to choose pixopher over
alternatives.

**D13: Small MCP tool surface built on the batch language** (2026-11-23)
About 8 tools instead of one per op. This keeps the model's context small,
and the script language is already the expressive layer.

**D14: Server binds to localhost only, with a token for writes** (2026-11-23)
The web viewer can trigger undo/restore, so a random token guards those
actions.

**D15: Distribution via go install, GitHub releases, Homebrew, Docker, deb/rpm/AUR** (2026-11-23)
`go install` from M0 and release binaries from M3 (goreleaser). The rest
comes in M11.

**D16: Undo reverts whole transactions, and steps are entries** (2026-11-23)
A `run` is one transaction (`tx` id). Each `--- step` inside it is its own
history entry with a rev and a caption, for narration. `undo` reverts the
whole transaction, and `undo --step` reverts one entry. This keeps "undo what
I just did" intuitive for agents while still giving viewers per-step
captions.

**D17: The default layer is `main`, and every doc has at least one layer and one frame** (2026-11-23)

**D18: Commit `.pxo.log` to git by default** (2026-11-23)
The history log is meant to be committed next to its `.pxo`. The docs
suggest `*.pxo.log -diff` in `.gitattributes`, so diffs and
reviews stay readable. `log --compact` handles logs that grow too large.
History stays in a sidecar, not a `.pixopher/` directory.
*Why*: losing history is irreversible, while deleting a committed log is one
command. The log is append-only, so git diffs are pure additions, and a
sprite session is typically a few hundred KB. A sidecar moves and renames
with its document; a central directory goes stale on `mv`.
*Consequences*: two branches that both append to a log conflict at its end.
This is a known limitation; merging pixel art across branches is rare.

**D19: MIT license** (2026-11-23)
Supersedes the license part of D11. pixopher is MIT-licensed.
*Why*: the license never covers artwork made with pixopher, and agents use it
as a subprocess or over MCP either way. It only matters to tools that import
the Go packages, such as engines or editors reading `.pxo`, and those are the
users who spread the format. Relicensing is trivial now, before any outside
contributions, and hard afterwards.
*Consequences*: dependencies must be MIT-compatible (permissive). This also
settles the GPL compatibility concern for the WebP encoder.

**D20: Undo is per author, as an appended compensating entry** (2026-11-23)
`undo` reverts the latest non-undone transaction by the current author
(`--as`, `PIXOPHER_AUTHOR`, default `cli`). `undo --any` reverts the latest
transaction by anyone. Undo appends an entry and never rewrites history
(see D16). If a later transaction by another author changed any of the same
pixels, undo fails with `undo_conflict` (exit 3), and `--json` lists the
overlapping pixels and transactions. `--force` reverts them anyway. `redo`
is per author too.
*Why*: with several agents on one file, "undo what I just did" must not
revert someone else's work. A single agent sees no difference, since every
entry has the same author. Partially reverting a transaction would leave the
drawing in a state nobody made, so conflicts are refused instead.
*Consequences*: every history entry records `author` from M0, even before
multiple authors are common.

**D21: Version info comes from ldflags, with a Go build info fallback** (2026-11-23)
`main.version`, `main.commit` and `main.date` are set with `-ldflags -X`
(the names goreleaser uses by default). Any value left unset is filled from
`runtime/debug.ReadBuildInfo`: the module version, `vcs.revision` and
`vcs.time`. Otherwise it is `dev`, `none` or `unknown`.
*Why*: `go install ...@latest` cannot pass ldflags, but it still embeds the
module version, so installed binaries report something useful.
*Consequences*: `version` has no `--json` yet. When the `--json` envelope
lands in M1, `version` gets it like every other command.

**D22: Target Go 1.27, built with toolchain 1.27.4** (2026-11-23)
`go.mod` says `go 1.27.0` and `toolchain go1.27.4`. This supersedes the
Go 1.25 target from M0.
*Why*: 1.27.4 is the latest release. Keeping the patch level in the
`toolchain` line, not the `go` line, means CI and development builds use
1.27.4, while anyone with an older 1.27.x can still `go install` without a
forced toolchain download.
*Consequences*: golangci-lint must be built with Go 1.27 or newer (v2.15.0
is). Raise the `toolchain` line for new patch releases.

**D23: Parse `.pxo` with BurntSushi/toml, serialize by hand** (2026-11-23)
`internal/pxofile` decodes with `github.com/BurntSushi/toml` and writes the
canonical form with its own serializer. This supersedes the
`pelletier/go-toml/v2` pick in architecture.md and settles the TOML library
open question.
*Why*: palette order defines palette indices, and decoding into a map loses
it. BurntSushi's `MetaData.Keys()` is a stable API that returns keys in
document order, and `MetaData.Undecoded()` finds unknown keys (D25). In
pelletier v2 this needs the `unstable` AST package. Exact output bytes need a
hand-written writer with either library.
*Consequences*: `meta.created` is decoded as an untyped value, because a
`time.Time` field goes through `UnmarshalText` and hides whether the value
had an offset.

**D24: Canonical `.pxo` layout** (2026-11-23)
Sections are written in this order: header, `[meta]`, `[palette]`,
`[ramps]`, `[symmetry]`, `[[layers]]`, `[[frames]]`, `[[tags]]`,
`[regions]`, `[anchors]`, `[variants.*]`, `[lint]`, `[parts.*]` and
`[[cels]]`, with cels last because they are the bulk of the file. One blank
line separates sections.
- **Field order**: fixed fields follow the order in
  [file-format.md](file-format.md). Name-keyed tables (ramps, regions,
  anchors, variants, parts, per-frame anchors) are sorted by key, and the
  palette keeps its order.
- **Defaults**: default values and empty sections are omitted. Some fields
  are always written: the header (`pixopher`, `size`, `rev`, `key_width`),
  a layer's `name`, a frame's `duration_ms`, a tag's `name`, `from` and `to`,
  and a cel's `layer`, `frame` and `origin`.
- **Palette**: `.` is always written first, whatever its position in the
  input.
- **Colors and times**: colors are lowercase hex, and `#rrggbb` when opaque.
  `created` is a TOML offset datetime in UTC. Readers reject values without
  an offset, since their meaning depends on the machine.
- **Lint**: `lint.disable` is sorted.
- **RLE**: "less than half the size" compares the RLE row text (tokens and
  separating spaces) with `w * h * key_width`.
*Why*: the example in file-format.md was inconsistent (one layer listed every
field, another only its name). Omitting defaults gives smaller files and
smaller diffs.

**D25: Unknown keys in a `.pxo` file are a parse error** (2026-11-23)
A key this build doesn't know fails with `parse_error`, naming the key.
*Why*: warning and then dropping the key on the next save would silently
destroy data written by a newer minor version. Rejecting also catches typos
in hand-edited files.
*Consequences*: an additive format change (same `pixopher = 1`) can't be
read by older builds. They report the unknown key, and the fix is to
upgrade.

**D26: Ramp base step in the file, default base** (2026-11-23)
A ramp is written as a bare key array when its base is the default. Otherwise
it is an inline table, `skin = { steps = ["S", "s", "t"], base = "s" }`, and
readers accept both forms. The default base of an n-step ramp is
`(n - 1) / 2`: the middle step, or the darker of the two middle steps.

**D27: Names are ASCII identifiers** (2026-11-23)
Palette entries, ramps, layers, tags, regions, anchors, parts and variants are
named with `[A-Za-z_][A-Za-z0-9_]*`, at most 64 characters. The color keywords
`transparent`, `none`, `key` and `name` are reserved. A palette entry and a
ramp may share a name (`skin`). Color syntax then picks the entry, and
`skin:0` / `skin+1` pick the ramp.
*Why*: names appear in CLI arguments, script lines and references (`%neck`,
`@cork`), where spaces and punctuation would need quoting. Relaxing the rule
later is compatible, while tightening it would not be.

**D28: Color resolution details** (2026-11-23)
- **Keys first**: a string exactly `key_width` characters long is a key
  first. With key width 1 this is the documented "single character is a key
  first" rule.
- **Hex reuse**: a hex color that exactly matches an entry resolves to that
  entry instead of creating an auto entry.
- **Transparent hex**: any hex with alpha 0 resolves to `.`. Palette
  entries with alpha 0 other than `.` are allowed, but can only be picked by
  key or name.
- **Hex forms**: only `#rgb`, `#rrggbb` and `#rrggbbaa` are accepted (no
  `#rgba`).
- **Resolution is pure**: `color.Resolve` never changes the document. For a
  new raw color it returns the color, and the op adds
  `Palette.AutoEntry(...)` as part of its undoable change.
- **Auto entries**: the key is the first free one in the order `a-z`, `A-Z`,
  `0-9`, then the other allowed punctuation in ASCII order (pairs in the same
  order for key width 2). The name is `auto_rrggbb`, with `aa` appended when
  the color isn't opaque and `_2`, `_3`, ... when the name is taken.
- **Widening**: switching a full key-width-1 palette to key width 2 is not
  automatic yet. `AutoEntry` returns `palette_full` (see open-questions.md).

**D29: `internal/errs` holds error codes, and pixels are palette indices** (2026-11-23)
- **`internal/errs`**: a leaf package with no internal imports. It defines
  the stable error codes from cli.md as `*errs.Error{Code, Message, Hint,
  Arg}` and maps codes to exit codes: `usage` → 1; `rev_conflict`, `locked`
  and `undo_conflict` → 3; `io`, `no_file` and `server_unavailable` → 4;
  every other code → 2.
- **Pixels**: in memory, a cel covers the whole canvas and stores palette
  indices (`uint16`). Index 0 is always `.`. Keys only exist in the file, so
  changing `key_width` never touches pixels.
- **Empty cels**: a cel without content is dropped on read and on write,
  together with links to it, because a missing cel already means empty.

**D30: Ops apply through a recording `Ctx`** (2026-11-24)
`Op.Apply(ctx *ops.Ctx, args ops.Args) error` replaces the `(Change, error)`
signature sketched in architecture.md. Ops write pixels with `ctx.Set`,
which records a `doc.PixelDelta` (first before, last after, dropped when
they end equal). Header fields (palette, ramps, ...) may be changed
directly: `NewCtx` snapshots the header and `Ctx.Finish` returns a
`doc.MetaDelta` when it changed. One `Ctx` covers one history step.
`PixelDelta` and `MetaDelta` live in `doc`, so `pxofile` can encode them
without importing `ops`.
*Why*: an op cannot forget to record a change, and every op gets undo for
free. Header snapshots are small, while copying cels for every step would
not be.
*Consequences*: Args hold parsed values (`doc.Point`, `int`, raw color
strings, ...). Text parsing and formatting of args come with cobra
generation. Until then a `Call` carries its normalized script line.

**D31: History entry undo encoding** (2026-11-24)
- **Pixels**: `undo.pixels` is base64 of a packed byte string. Byte 0 is
  the version, `1`. Then, for each cel: layer, frame, canvas width and run
  count, followed by runs of consecutive row-major pixels that share the
  same before and after value. Each run is the gap from the end of the
  previous run, the length, before and after. All numbers are unsigned
  varints. No pixel deltas give `""`. Both directions are stored, so one
  entry supports undo, redo and the D20 overlap check.
- **Meta**: `undo.meta` is `[]`, or one `{"before": ..., "after": ...}`
  pair holding the canonical `.pxo` text of the document header (no cels,
  `rev = 0`) before and after the step.
*Why*: runs keep a full 64x64 fill at 16 base64 characters instead of tens of KB, which
matters for logs that are committed to git (D18). Header snapshots reuse
the `.pxo` format, so every current and future meta edit (palette, ramps,
layers, regions, ...) is undoable without a codec per section.
*Consequences*: a meta-changing entry is a few hundred bytes to a few KB.
Undoing a meta change while another author has since changed the header is
an `undo_conflict` (D20) unless forced, because the whole header is
restored.

**D32: Direct-mode transaction details** (2026-11-24)
- **Rev and tx**: every step that changes something bumps `rev` and writes
  one entry. A step that changes nothing writes no entry. A transaction that
  changes nothing writes neither the file nor the log, and keeps its rev.
  The tx id is `t_` and the rev of the transaction's first entry,
  zero-padded to four digits (`t_0012`). No log read is needed to make one.
  Revs only grow while the document and its log move together; a rollback
  of the document alone (hand edit, `git checkout`) can reuse a tx id (see
  open-questions.md).
- **Atomicity**: a failing call aborts the whole transaction before
  anything is written. The document is written first (temp file in the same
  directory, fsync, rename, permissions kept), then the log is appended in
  one write. The log and the directory are not fsynced. On macOS the fsync
  is a plain `fsync`, not `F_FULLFSYNC`. A full command on a 64x64 document
  takes about 3 ms on Linux.
- **Lock**: `<name>.pxo.lock` is retried every 5 ms for up to 2 s, then the
  command fails with `locked`. The lock file is never deleted, because
  deleting it while another process waits would let two writers in.
  Readers take no lock. A missing document fails with `no_file` before
  the lock file is created.
- **Dry run**: takes no lock and writes nothing, but runs the same checks
  (including `--expect-rev`) and reports the would-be rev with
  `"dry_run": true`.
- **Output**: `bbox` is omitted when no pixel changed. `changed` counts
  distinct pixels that differ from the start of the transaction.
- **Log tail**: a partial last line (a crash during append) is ignored by
  readers and cut off by the next append. A complete last line that only
  lacks its newline is kept.

**D33: `golang.org/x/sys` for file locks** (2026-11-24)
`flock` (`unix.Flock`) on Linux, macOS and the BSDs, and `LockFileEx` on
Windows, come from `golang.org/x/sys`. The standard library has no
`LockFileEx`. The license is BSD-3-Clause, which is compatible with D19.
Other platforms (such as wasm) get no lock; `--expect-rev` still detects
conflicting writes.

**D34: Argument text forms and canonical script lines** (2026-11-25)
`ops.Parse` (`Split` then `Build`) turns tokens into `Args`, and
`ops.Format` turns `Args` back into the canonical line stored in
`Call.Line` and the log's `ops`. The CLI uses them now, and batch scripts
will use the same functions.
- **Values**: points `x,y`; sizes `w,h` or `wxh`, both at least 1; rects
  `x,y,w,h`, or `x,y w,h` as a positional; ints in decimal; bools
  `true`/`false` (anything `strconv.ParseBool` reads); enums must match a
  listed value exactly; colors are kept as raw text and resolved by the op.
- **Canonical form**: the op name, then positionals in spec order, then flags
  in spec order. Arguments at their default are left out, and so are
  trailing optional positionals. Sizes are written `w,h` and rects
  `x,y,w,h`. The one-token rect form is the only one that works as a flag
  value. Bools are written `--fill`, or `--fill=false` when the default is
  true. Strings that need it get shell-style single quotes. Common flags
  (`-m`, `--as`, ...) are never part of the line.
- **Defaults**: `Build` fills in missing arguments that have a `Default`, so
  ops never repeat a default. `Register` rejects defaults that don't parse,
  bool positionals, a default on a required argument, and short flags that
  are digits or longer than one letter.
- **Errors**: malformed text, missing or extra positionals, unknown flags
  and flags given twice are `usage` errors (exit 1) with `arg` set when a
  declared argument is at fault (not for unknown flags or extra
  positionals).
*Why*: logged lines are replayed and shown as captions, so one spelling per
call keeps logs diffable and replays identical. A rect written as one token
works both as a positional and as a flag.

**D35: `use` is a plain command, not an op** (2026-11-25)
`use <file>` changes no document, so it doesn't fit `Op.Apply` and is
hand-written like `version`. It is still listed under the roadmap's M1 ops.
It fails with `no_file` when the file is missing or is a directory. It
stores the path cleaned, with forward slashes, plus a newline, in
`.pixopher/current`. A relative path is relative to the working directory,
which is also the only place resolution looks for `.pixopher/current`.
Resolution ignores an empty current file. It fails with `no_file` when the
stored file no longer exists, rather than falling through to the
only-`*.pxo` rule.
*Why*: a stale `use` silently switching to another file would make edits
land in the wrong document.

**D36: CLI output and the error envelope** (2026-11-25)
- With `--json`, stdout gets exactly one JSON line, for success and for
  errors alike, and nothing is written to stderr. HTML characters are not
  escaped.
- Without `--json`, errors go to stderr as `error: ...` and `hint: ...`.
- `errs.Error` serializes as `{"code", "message", "hint"?, "arg"?}`.
- Errors that aren't `*errs.Error` come from cobra (unknown command, wrong
  argument count, bad flag on a cobra-parsed command) and become `usage`.
  Those from the engine's file system calls become `io`.
- When flags were never parsed (an unknown command or a bad flag), the raw
  arguments are scanned for `--json` to choose the format.
- `version --json` is `{"ok", "version", "commit", "date"}`, which settles
  D21's consequence. `use --json` is `{"ok", "file"}`.
*Why*: agents read stdout. Getting one parseable line whatever happened
means they never have to merge two streams.

**D37: Generated commands parse their own flags** (2026-11-25)
Generated op commands set cobra's `DisableFlagParsing` and parse every
token with `ops.Split`. The common flags are added to the op's own. The
pflag definitions on the command only feed `--help`, and `-h`/`--help`
before `--` shows help. `--json` stays a persistent root flag for the other
commands. An op flag that reuses a common flag's name or short letter, or
`help`/`h`, makes the generator panic. So does an op with `Mutates: false`,
until observation ops get an output channel (see open-questions.md).
*Why*: pflag reads `-2,-2` as the shorthand flag `-2`, so negative
coordinates would need `--`. Parsing with the same code that batch scripts
will use also keeps the two interfaces from drifting apart.
*Consequences*: shell completion doesn't complete the flags of op commands.

**D38: Observation ops implement a separate read-only interface** (2026-11-25)
Ops that only look at a document (`info`, `show`, `get`, `stats`, later
`lint`, `diff`, ...) implement their own interface instead of `Op.Apply`:
they get the loaded document and their `Args` and return a report, which
has a JSON form for `--json` and a human form. Their spec has
`Mutates: false`. They take no lock and never go through a transaction, so
they can't write, bump the rev or log. The exact Go shape lands with
`info`. This settles the open question from D37, and the generator accepts
these ops from then on.
*Why*: a read-only op that can't write by construction is safer than one
trusted not to. A typed report gives a stable `--json` shape per command.
*Consequences*: the generator, `schema` and MCP handle two kinds of op.

**D39: `new` is a registry op marked as creating its document** (2026-11-25)
A spec flag (`Creates`) marks ops that make a document instead of applying
to one. For them the generator skips file resolution and the
leading-`.pxo` rule: the file is the op's own required positional. The
engine gets a create path that fails with `file_exists` unless `--force`
is given, and writes the new document (rev 0) with the same atomic write.
*Why*: `new` is the first command an agent runs, so it must appear in
`schema`, the guide and MCP like every other op. A hand-written command
would hide it from them.
*Consequences*: one special case in the generator and the engine.

**D40: Trailing repeat groups** (2026-11-25)
All trailing positionals marked `Repeat` form one group that repeats
together: `px <at> <color> [<at> <color> ...]`. Each one's value is a
slice, and all of them have the same length. A required group needs at
least one full repetition, and a partial last repetition is a `usage`
error. Repeated positionals followed by fixed ones (`polyline`, `poly`)
are left for M2. This supersedes "only the last positional may repeat".
*Why*: it keeps `px` as cli.md describes it (a color per pixel) without
building M2's grammar early.

**D41: The canonical form writes `--` before positionals that look like flags** (2026-11-25)
When a positional's text starts with `-` and a non-digit, `ops.Format`
writes the flags first, then `--`, then the positionals, so the line parses
back the same. Lines without such a value don't change. Implemented with
the batch parser in M2; no op takes such a value before then.

**D42: Argument defaults are public API** (2026-11-25)
Logged lines leave defaults out (D34), so a replay of an old log uses the
defaults of the binary replaying it. Changing a default is therefore a
breaking change, like renaming an error code.
*Why*: it keeps logs and captions short, which matters for committed logs
(D18) and live captions, and keeps replays stable as long as defaults hold.
Logging every argument would be safer but would bury each line in
defaults.
*Consequences*: when `schema --json` lands (M8), a golden test of it
guards the defaults of every op.

**D43: Color errors are domain errors** (2026-11-25)
Colors stay raw text until the op resolves them, and `invalid_color` and
`unknown_color` are domain errors (exit 2), even for a typo like `#ggg`.
This is the one exception to D34's "malformed text is `usage`".
*Why*: whether a text is a key, a name or a ramp step depends on the
document's key width and palette (D28), so a color can't be judged without
the document.

**D44: Human error lines show the code** (2026-11-25)
Without `--json`, an error prints `error[<code>]: <message>`, then
`hint: <hint>` when there is one. This supersedes the `error: ...` form in
D36. With `--json`, errors still go to stdout as one line (D36 is
unchanged there).
*Why*: people can search for the code and match it against the docs, and it
lines up with batch script errors, which also lead with a code.
