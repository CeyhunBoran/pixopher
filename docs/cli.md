# CLI specification

The binary is `pixopher`. The release packages also install `pxo` (same binary).
Examples use `pxo`.

## Conventions

### File resolution [done]

Most commands act on one document. The document is chosen in this order:

1. `-f, --file <path>` flag, or the first positional argument when it ends in
   `.pxo` (`pxo rect hero.pxo 1,1 4,4 ink` is valid). Giving both is a
   `usage` error,
2. `PIXOPHER_FILE` environment variable,
3. `pxo use <path>`, which stores the current file in
   `.pixopher/current` in the working directory,
4. the only `*.pxo` file in the working directory, if there is exactly one.

Otherwise the command fails with `no_file`, and the message lists the
candidates in name order. The `.pxo` suffix is matched case-sensitively
everywhere. If the file stored by `use` no longer exists, the command fails
with `no_file` and says so (D35).

### Coordinates and geometry

- Origin is the **top-left**. `x` grows right, `y` grows down, and both are
  0-based.
- Point: `x,y`, e.g. `3,4`. Negative values are allowed where clipping makes
  sense (shapes may extend off-canvas and are clipped; `px` out of bounds is
  an error).
- Size: `w,h` or `wxh`. Sizes and rect widths and heights are at least 1.
  Every number in a point, size or rect is within ±1048576 (D48).
- Rect: `x,y w,h` (two args) or `x,y,w,h` (one arg). A flag value takes the
  one-arg form.
- Area (`erase`): a rect, or one pixel `x,y` (D47).
- Anchor reference: `@cork`, with an offset `@cork+1,-2`.
- Region reference: `%flask` (a rect). `%flask.center`, `%flask.tl`, `.br`
  etc. give points.
- `--layer <name|index>` and `--frame <index|tag:name>` choose the target. The
  default is the *active* layer/frame, which is stored in the document's
  `[meta]` and set by `layer use` and `frame use`.

### Colors

See [palettes.md](palettes.md#color-syntax-everywhere-a-color-is-accepted-done).

### Common flags (all mutating commands)

| Flag | Meaning |
| --- | --- |
| `-f, --file <path>` | the document (see [file resolution](#file-resolution-done)) |
| `-m, --message <text>` | caption for this transaction (shown live, stored in the log) |
| `--as <author>` | author label (default `$PIXOPHER_AUTHOR` or `cli`) |
| `--expect-rev <n>` | fail with `rev_conflict` unless the doc is at rev n |
| `--dry-run` | compute and report the change without writing |
| `--no-sym` | ignore the document's symmetry mode for this command [M2] |
| `--mask <region\|layer>` | only affect pixels inside the mask [M2] |
| `--json` | machine-readable output (every command, not only mutating ones) |
| `--no-server` | don't forward to a running `serve` daemon [M7] |

`-f`, `-m`, `--as`, `--expect-rev`, `--dry-run` and `--json` are
**[done]**. `new` creates its document instead of changing one, so it
takes only `--dry-run`, `--json` and its own `--force` (D46). The others arrive with the features they control. An op can't
define a flag with the same name or short letter as a common flag.

Flags may appear anywhere on the command line, before or after the
positionals. A token that starts with `-` and a digit is a value, not a
flag, so negative coordinates need no quoting (`pxo rect -2,-2 5,5 ink`).
Everything after `--` is positional. A flag given twice is a `usage` error.

### Output [done]

Human output is short by default: `ok rev 13  +24px  bbox 6,1 4,3`.
A command that changes nothing prints `ok rev 12  +0px`, and a dry run
appends `  (dry run)`. Warnings go to stderr as `warning: ...`.

With `--json`, stdout gets exactly one line of JSON:

```json
{"ok": true, "rev": 13, "changed": 24, "bbox": [6, 1, 4, 3], "warnings": []}
```

`changed` counts the distinct pixels that differ from before the command,
and `bbox` is omitted when no pixel changed. A command that changes nothing writes
nothing and keeps the rev. With `--dry-run`, `rev` is the rev the command
would produce and the output adds `"dry_run": true`.

On error, with `--json`, the error envelope goes to **stdout** and nothing
goes to stderr:

```json
{"ok": false, "error": {"code": "out_of_bounds", "message": "x=40 is outside the canvas", "hint": "canvas is 32x32; valid x is 0..31", "arg": "at"}}
```

`hint` and `arg` are omitted when empty. Without `--json`, stderr gets
`error[<code>]: <message>` and, when there is one, `hint: <hint>`
(D44). A `usage` error
without its own hint gets ``see `pixopher <command> --help` ``.

A malformed argument (`1;1` for a point, an unknown enum value, a missing
or extra positional, an unknown flag) is a `usage` error. `arg` names the
argument when it is one the command declares, so it is absent for an
unknown flag or an extra positional. A well-formed value the document
rejects (off the canvas, an unknown color) is a domain error with its own
code.

Colors are the exception to the "malformed is `usage`" rule. Whether a
color text is a key, a name or a ramp step depends on the document (its key
width and palette), so colors are checked against the document, and both
`invalid_color` and `unknown_color` are domain errors (exit 2), even for a
typo (D43).

`PIXOPHER_JSON` turns `--json` on globally, which is handy for agents. Any
value Go's `strconv.ParseBool` reads as true counts (`1`, `t`, `true`,
...). An explicit `--json=false` still wins.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | usage error (bad flags, unknown command) |
| 2 | domain error (bounds, color, missing layer, ...) |
| 3 | conflict (`rev_conflict`, `locked`, `undo_conflict`) |
| 4 | I/O error |
| 5 | lint findings at or above `--fail-on` severity |

### Error codes

`usage`, `no_file`, `file_exists`, `parse_error`, `unsupported_version`,
`out_of_bounds`, `invalid_color`, `unknown_color`, `unknown_layer`,
`unknown_frame`, `unknown_region`, `unknown_anchor`, `unknown_part`,
`layer_locked`, `rev_conflict`, `locked`, `undo_conflict`, `no_history`, `too_large`,
`palette_full`, `grid_mismatch`, `script_error` (wraps another error and adds
`line`), `io`, `unsupported_format`, `server_unavailable`.

Codes are stable. They are part of the public API, and adding one is a minor
change. Argument defaults are public API too: logged lines leave defaults
out, so changing a default is a breaking change (D42).

## Command reference

Groups and commands. `[M#]` is the roadmap milestone that delivers the command.

### Document

```
pxo new <file> [--size 32x32] [--bg <color>] [--force]   [done]  (--palette <name|file|none> comes with the built-in palettes, D46)
pxo info [--json]                     size, rev, palette size, layers, frames, regions, anchors   [M1]
pxo use <file>                        set current file for this directory   [done]
pxo resize <w,h> [--anchor tl|c|...]  change canvas size, keeping pixels   [M6]
pxo crop <rect|%region|auto>          crop canvas (auto = content bbox)   [M6]
pxo clear [color]                     clear target cel, or fill it with color   [done]
pxo migrate                           upgrade old format versions   [later]
```

### Palette, ramps, variants

See [palettes.md](palettes.md). The `palette`, `ramp`, `variant` and
`recolor` groups.

### Drawing [M1–M2]

```
pxo px <x,y> <color> [<x,y> <color> ...]        set pixels   [done]
pxo line <x,y> <x,y> <color> [--width 1]         Bresenham line, no L-corners by construction (D45)   [done]
pxo polyline <x,y> <x,y> ... <color> [--closed] [--raw]
pxo rect <x,y> <w,h> <color> [--outline]        filled unless --outline   [done]
pxo ellipse <cx,cy> <rx,ry> <color> [--fill|--outline]
pxo circle <cx,cy> <r> <color> [--fill|--outline]
pxo poly <x,y> <x,y> <x,y> ... <color>            filled polygon (even-odd)
pxo fill <x,y> <color> [--contiguous=true] [--8]   [done]  ([M4] --sample layer|composite)
pxo erase <rect|%region|x,y>                     to transparent   [done]  ([M2] %region)
pxo replace <from> <to>                          [done]  ([M2] --region %r)
pxo grid <x,y> [--legend "a=skin b=ink"] <<'EOF' ... EOF    paint a letter grid (see agent-workflow.md)
pxo stamp <part> <x,y|@anchor> [--flip h|v] [--rot 90|180|270] [--align tl|c|bc]
pxo gradient <rect> <ramp> [--dir down|right|...] [--dither bayer4|checker|none]
pxo dither <rect|%r> <colorA> <colorB> [--pattern checker|bayer2|bayer4|bayer8] [--level 0.5]
```

### Symmetry and transforms [M2, M6]

```
pxo sym <off|x|y|xy> [--axis-x 16] [--axis-y 16]     document symmetry mode (mirrors every draw op)
pxo mirror <h|v> [--region %r] [--from left|right|top|bottom]   copy one half onto the other
pxo flip <h|v> [--layer L | --region %r | --all]
pxo rotate <90|180|270> [...]
pxo shift <dx,dy> [--wrap] [...]                 nudge pixels
pxo shear <h|v> <k> [...]                        "lean" by k px per row/col step
pxo scale <n> [...]                              integer nearest-neighbor upscale of a layer/region
```

### Parts, regions, anchors [M2]

```
pxo part save <name> <rect|%region> [--layer L]      capture pixels as a reusable part
pxo part add <name> <<'EOF' ... EOF                  define from a letter grid
pxo part list | show <name> | rm <name>
pxo region set <name> <rect> | rm <name> | list
pxo anchor set <name> <x,y> [--frame n] | rm <name> | list
```

### Style effects [M6]

```
pxo outline [--color ink|auto] [--outer|--inner] [--corners] [--selective] [--layer L] [--to-layer L2]
pxo shade [--light top-left|top|...|<angle>] [--layer L] [--strength 1] [--style soft|hard|rim] [--ramp-only]
pxo highlight [--light ...] [--layer L]           one-pixel rim highlights on the lit edges
pxo aa [--layer L]                                 manual-style anti-aliasing on jaggy curves using ramp midtones (optional pass)
```

Details are in [drawing.md](drawing.md).

### Layers and frames [M4]

```
pxo layer add <name> [--below L|--above L] | rm | rename | mv <name> <index> | dup
pxo layer use <name> | hide | show | solo | lock | unlock | opacity <0-255> | blend <normal|multiply|add|screen>
pxo layer merge <upper> [into <lower>] | flatten
pxo frame add [--after n] [--dup] | rm <n> | mv <n> <to> | use <n> | duration <n> <ms>
pxo tag add <name> <from>-<to> [--dir pingpong] | rm | list
pxo onion <on|off> [--before 1 --after 1]          viewer setting stored in doc meta
```

### Observation (non-mutating)

```
pxo show [--grid|--ansi|--legend] [--region %r] [--layer L|--composite] [--frame n] [--rulers]   [M1]
pxo get <x,y> [--layer L|--composite]               color at a pixel: key, name, hex   [M1]
pxo stats                                            color counts, bbox, coverage   [M1]
pxo diff [<rev>|<file>] [--grid]                     what changed since rev/file   [M5]
pxo look [-o look.png] [--zoom 8] [--since-look]     preview sheet for multimodal agents   [M5]
pxo lint [--rule r] [--fail-on warn] [--json]        see lint.md   [M5]
pxo schema [--json]                                  all op specs   [M8]
pxo guide [topic]                                    agent manual   [M8]
```

### Batch

```
pxo run [<script>|-] [-m msg] [--step-messages] [--continue-on-error]   [M2]
```

Details are in [agent-workflow.md](agent-workflow.md#batch-scripts).

### History [M1, M7]

```
pxo undo [n] [--step] [--any] [--force] | redo [n] [--step]   own last n transactions (a whole `run`), or n steps with --step; --any: anyone's   [M1]
pxo log [--limit 20] [--json]                    [M1]
pxo checkpoint <name> | restore <name|rev>       [M7]
pxo replay [--from rev] [--speed 2x] [--tui]     [M7]
```

### Live and integration

```
pxo watch [file] [--mode auto|halfblock|kitty|sixel|ascii]   [M3]
pxo serve [files...] [--port 7777] [--open]                 [M7]
pxo mcp [--root dir]                                        [M8]
```

### Import and export [M1, M4, M9]

```
pxo export <out.(png|svg|gif|jpg|webp|ico|json)> [--scale 4] [--frame n|--tag idle|--all-frames]
           [--layer L|--composite] [--variant v|--variants all] [--sheet grid|row|col --cols 8 --atlas out.json]
           [--bg color] [--quality 95] [--sizes 16,32,48]
pxo export --timelapse out.gif [--fps 12] [--from rev]                        [M7]
pxo import <in.png> [<new.pxo>] [--palette keep|quantize:16|doc] [--as-layer ref --locked] [--pixelate 32x32]
```

Details are in [export-import.md](export-import.md).

### Misc [done]

```
pxo version | completion <bash|zsh|fish|powershell> | help <cmd>   [M0]
```

`version` prints one line, `pixopher <version> (commit <sha>, built <date>)`.
With `--json` it prints
`{"ok": true, "version": "...", "commit": "...", "date": "..."}`. `use`
prints `using <file>`, or `{"ok": true, "file": "<file>"}` with `--json`,
where `<file>` is the stored path (D35).
Release builds set the three values with
`-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`. Without
ldflags, they come from the module version and VCS stamps that Go embeds in
the binary, and fall back to `dev`, `none` and `unknown` (see D21).
