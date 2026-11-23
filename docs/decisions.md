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
