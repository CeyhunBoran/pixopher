# Roadmap

Milestones are in **build order**. Each one is shippable on its own and has
acceptance criteria that define "done". Tick the boxes as they land.

Versioning: `0.x` until M11. Releases are tagged at the end of M3 (0.1), M5
(0.2), M7 (0.3), M8 (0.4), M9 (0.5) and M10 (0.6). M11 ships **1.0**.

---

## M0: Bootstrap

Goal: an empty but correctly structured Go project with CI.

- [x] `go.mod` as `github.com/CeyhunBoran/pixopher`, Go 1.27 (toolchain 1.27.4)
- [x] `cmd/pixopher/main.go` with a cobra root, `version`, and `completion`
- [x] Package skeleton from [architecture.md](architecture.md#package-layout) (only the packages M1 needs)
- [x] CI (GitHub Actions): `go vet`, `golangci-lint`, `go test -race ./...` on linux, macOS and windows
- [x] `go install github.com/CeyhunBoran/pixopher/cmd/pixopher@latest` works
- [x] Root `README.md` with the pitch and status

**Done when** CI is green on all three OSes and `pixopher version` prints the
version, commit and date (set via ldflags).

## M1: Core document

Goal: create, draw basic shapes, read back, export PNG, undo. The data model
already includes layers and frames, but only one of each is exposed.

- [x] `internal/doc` model with palette, ramps, layers, frames, cels, regions and anchors (fields exist, most are unused yet)
- [x] `.pxo` parse + canonical serialize, grid + RLE cels ([file-format.md](file-format.md))
- [x] Color syntax: key, name, hex, ramp step, transparent ([palettes.md](palettes.md))
- [x] Engine direct mode: lock, atomic write, rev, `--expect-rev`, `--dry-run` (engine options; the flags come with cobra generation)
- [x] File resolution (`-f`, `PIXOPHER_FILE`, `use`, the only `*.pxo`)
- [x] History log JSONL: entries with tx ids and undo deltas, written and read
- [x] `undo`, `redo`, `log` (D49, D50)
- [x] Op registry core types: spec, args, recording ctx, change, registry
- [x] Cobra generation from specs for mutating ops (arg parsing and canonical lines, common flags, `--json` and the error envelope); observation ops come with `info`
- [x] Ops: `new`, `use`, `clear`, `px`, `line` (pixel-perfect), `rect`, `fill`, `erase`, `replace`
- [x] Op: `info` (the first observation op, D38, D52)
- [x] `palette list/add/set/rename/rm`, `ramp add/rm`; built-in `default`; `new --palette`, with `default` when it is not given and `none` for an empty palette (D46)
- [x] Palette-aware undo: compare palette entries one by one instead of the whole header (D31, D49), so another author's new color doesn't block undo, and settle forced restores that reuse a palette index (D54, [decisions.md](decisions.md))
- [x] Observation: `show --grid --rulers --legend`, `get`, `stats` (D55)
- [x] `export` PNG with `--scale`, indexed when ≤256 colors (D58)
- [ ] `--json` output, error codes and exit codes from [cli.md](cli.md)
- [ ] Golden tests, round-trip tests, and undo-identity tests for every op

**Done when** an agent can run `new → px/line/rect/fill → show --grid →
export --scale 8` and `undo` back to an empty canvas, with every step's
`--json` matching the spec. A 64x64 single command runs in under 15 ms.

## M2: Batch, grids, structure helpers

Goal: agents can draw a lot per call and refer to things by name.

- [ ] `run` with steps (`---`), `@set`, inline grid blocks, line-numbered errors, `--pace`
- [ ] `grid` paint with `--legend`, `--opaque`, `_` erase
- [ ] `ellipse`, `circle`, `poly`, `polyline`, `gradient`, `dither`
- [ ] `sym` (x/y/xy, .0/.5 axes) and `mirror`
- [ ] `part save/add/list/show/rm`, `stamp` with flip/rot/align
- [ ] `region` / `anchor` commands and `%`/`@` references everywhere
- [ ] `--mask`

**Done when** a symmetric 32x32 character can be drawn in at most 5 `run`
calls, using only the left half, regions, anchors and stamped parts. A
golden test covers exactly that script.

## M3: Live terminal viewer → release 0.1

Goal: the human can watch.

- [ ] `watch` with halfblock + ascii renderers, auto-fit, zoom/pan, checkerboard
- [ ] fsnotify-based updates, caption bar from the log, stroke flash
- [ ] kitty graphics renderer with capability detection, and halfblock fallback
- [ ] Viewer undo/redo (`--read-only` to disable)
- [ ] goreleaser config, GitHub release binaries (linux/macos/windows × amd64/arm64), `pxo` alias in archives
- [ ] `docs` updated with real screenshots/GIFs

**Done when** `watch` in one pane updates within 100 ms of each command in
another pane, in alacritty (halfblock) and kitty (graphics), and the 0.1
release is downloadable.

## M4: Layers, frames, animation export

- [ ] `layer` and `frame` command groups, `tag`, opacity/blend, linked cels
- [ ] Compositing in `render`
- [ ] `--layer` and `--composite` for `show`, `get` and `stats`, with blended pixels as `+` (D57)
- [ ] `watch`: layer solo, animation playback, onion skin
- [ ] Export: GIF, spritesheet + JSON atlas (incl. aseprite-compatible), SVG, JSON dump
- [ ] Output path placeholders and multi-input export

**Done when** a 4-frame idle animation with 3 layers exports to GIF, sheet +
atlas and SVG (per frame), all covered by golden tests.

## M5: Seeing and judging → release 0.2

- [ ] `look` preview sheet (zooms, backgrounds, silhouette, changes since last look, frames strip)
- [ ] `diff` (rev or file, `--grid`)
- [ ] `lint` with the rules in [lint.md](lint.md) (in this order: palette, orphan, doubled-line, jaggy, stray-alpha, symmetry-break, then the rest)
- [ ] `lint --fix` for fixable rules, `--fail-on`
- [ ] `look --critique`

**Done when** every lint rule has positive and negative fixtures, and `look`
output for a fixture matches its golden PNG.

## M6: Style effects and transforms

- [ ] `outline` (outer/inner, corners, selective, to-layer, each)
- [ ] `shade` (soft/hard/rim) and `highlight`, `[meta] light`
- [ ] `ramp gen` (OKLCH, hue shift)
- [ ] `variant` and `recolor`, `export --variants`
- [ ] Transforms: `flip`, `rotate`, `shift --wrap`, `shear`, `scale`, `resize`, `crop`, `--move-markers`
- [ ] `aa` pass (optional, can slip to later)

**Done when** a flat-colored sprite goes through `outline --selective` +
`shade --light top-left` and produces the golden result, and `export
--variants all` writes one file per variant.

## M7: Server, web viewer, replay → release 0.3

- [ ] `serve`: socket RPC, CLI auto-forwarding, write-through, external-edit reload
- [ ] Event stream (SSE) and `--pace` live ops
- [ ] Embedded web viewer: live canvas, timeline scrubber, animation player, side panels, multi-file tabs, token for writes
- [ ] `checkpoint` / `restore`, `replay`, `export --timelapse` (captions, per-step, per-pixel, duration target)
- [ ] `watch` connects to the server when available

**Done when** the CLI gives the same output with and without `serve` (one
test suite run in both modes), and a timelapse GIF of an M2 golden script
matches its golden file.

## M8: Agent integration → release 0.4

- [ ] `schema --json` from the registry, plus the engine commands `undo`, `redo`, `log` and `export`, which are not ops (D50, D58)
- [ ] `guide` and topics, `guide --install`, craft notes; the guide covers `undo`, `redo` and `log` too (D50)
- [ ] `mcp` stdio server with the tools and resources in [agent-workflow.md](agent-workflow.md#mcp-server), `--root` sandboxing
- [ ] MCP `look` returns inline image content

**Done when** an MCP client can create, draw, look, lint and export a sprite
using only MCP tools, with viewers updating live.

## M9: More formats and import → release 0.5

- [ ] JPEG (flatten + warning), WebP lossless, ICO multi-size
- [ ] `import` PNG/GIF/JPEG/WebP with `keep` / `quantize` / `doc` palettes, `--pixelate`, `--detect-scale`, `--frames`
- [ ] Reference layers (locked, excluded from export), `--ramps auto`
- [ ] `palette load/save` for `.hex .gpl .pal .ase`
- [ ] `palette widen`, and width-2 keys for new documents that need over 82 colors (D56)

## M10: Libraries and helpers → release 0.6

- [ ] Built-in palette library with suggested ramps and attributions
- [ ] Animation helpers: `tween`, `anim breathe/bob/blink`, `frame shift-layer`
- [ ] Tile mode: wrap drawing, 3x3 preview, `lint seam`, `tileset autotile`

## M11: Distribution and 1.0

- [ ] Homebrew tap (goreleaser), Docker image (headless, for CI agents)
- [ ] `.deb` / `.rpm` (goreleaser nfpm) and AUR package
- [ ] Format version frozen at `pixopher = 1`, with a stability promise for the CLI, error codes and atlas JSON
- [ ] Full docs pass: every `[planned]` marker is `[done]` or moved to a post-1.0 list

## Post-1.0 ideas

Parts with color slots, style presets, sprite templates, `.aseprite` import,
animated WebP, RotSprite rotation, `lint --compare ref`, a plugin hook for
custom lint rules.
