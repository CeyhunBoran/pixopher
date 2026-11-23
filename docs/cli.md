# CLI specification

The binary is `pixopher`. The release packages also install `pxo` (same binary).
Examples use `pxo`.

## Conventions

### File resolution

Most commands act on one document. The document is chosen in this order:

1. `-f, --file <path>` flag, or the first positional argument when it ends in
   `.pxo` (`pxo rect hero.pxo 1,1 4,4 ink` is valid),
2. `PIXOPHER_FILE` environment variable,
3. `pxo use <path>`, which stores the current file in
   `.pixopher/current` in the working directory,
4. the only `*.pxo` file in the working directory, if there is exactly one.

Otherwise the command fails with `no_file`, and the message lists the
candidates.

### Coordinates and geometry

- Origin is the **top-left**. `x` grows right, `y` grows down, and both are
  0-based.
- Point: `x,y`, e.g. `3,4`. Negative values are allowed where clipping makes
  sense (shapes may extend off-canvas and are clipped; `px` out of bounds is
  an error).
- Size: `w,h` or `wxh`.
- Rect: `x,y w,h` (two args) or `x,y,w,h` (one arg).
- Anchor reference: `@cork`, with an offset `@cork+1,-2`.
- Region reference: `%flask` (a rect). `%flask.center`, `%flask.tl`, `.br`
  etc. give points.
- `--layer <name|index>` and `--frame <index|tag:name>` choose the target. The
  default is the *active* layer/frame, which is stored in the document's
  `[meta]` and set by `layer use` and `frame use`.

### Colors

See [palettes.md](palettes.md#color-syntax-everywhere-a-color-is-accepted).

### Common flags (all mutating commands)

| Flag | Meaning |
| --- | --- |
| `-m, --message <text>` | caption for this transaction (shown live, stored in the log) |
| `--as <author>` | author label (default `$PIXOPHER_AUTHOR` or `cli`) |
| `--expect-rev <n>` | fail with `rev_conflict` unless the doc is at rev n |
| `--dry-run` | compute and report the change without writing |
| `--no-sym` | ignore the document's symmetry mode for this command |
| `--mask <region\|layer>` | only affect pixels inside the mask |
| `--json` | machine-readable output |
| `--no-server` | don't forward to a running `serve` daemon |

### Output

Human output is short by default: `ok rev 13  +24px  bbox 6,1 4,3`.

With `--json`:

```json
{"ok": true, "rev": 13, "changed": 24, "bbox": [6, 1, 4, 3], "warnings": []}
```

`changed` counts the distinct pixels that differ from before the command,
and `bbox` is omitted when no pixel changed. A command that changes nothing writes
nothing and keeps the rev. With `--dry-run`, `rev` is the rev the command
would produce and the output adds `"dry_run": true`.

On error:

```json
{"ok": false, "error": {"code": "out_of_bounds", "message": "x=40 is outside the canvas", "hint": "canvas is 32x32; valid x is 0..31", "arg": "x"}}
```

`PIXOPHER_JSON=1` turns `--json` on globally, which is handy for agents.

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
change.

## Command reference

Groups and commands. `[M#]` is the roadmap milestone that delivers the command.

### Document

```
pxo new <file> --size 32x32 [--palette <name|file>] [--bg <color>] [--force]   [M1]
pxo info [--json]                     size, rev, palette size, layers, frames, regions, anchors   [M1]
pxo use <file>                        set current file for this directory   [M1]
pxo resize <w,h> [--anchor tl|c|...]  change canvas size, keeping pixels   [M6]
pxo crop <rect|%region|auto>          crop canvas (auto = content bbox)   [M6]
pxo clear [color]                     clear target cel   [M1]
pxo migrate                           upgrade old format versions   [later]
```

### Palette, ramps, variants

See [palettes.md](palettes.md). The `palette`, `ramp`, `variant` and
`recolor` groups.

### Drawing [M1–M2]

```
pxo px <x,y> <color> [<x,y> <color> ...]        set pixels
pxo line <x,y> <x,y> <color> [--width 1]         Bresenham line ("pixel-perfect" mode on by default, no L-corners)
pxo polyline <x,y> <x,y> ... <color> [--closed]
pxo rect <x,y> <w,h> <color> [--fill|--outline]  default --fill
pxo ellipse <cx,cy> <rx,ry> <color> [--fill|--outline]
pxo circle <cx,cy> <r> <color> [--fill|--outline]
pxo poly <x,y> <x,y> <x,y> ... <color>            filled polygon (even-odd)
pxo fill <x,y> <color> [--contiguous=true] [--sample layer|composite]
pxo erase <rect|%region|x,y>                     to transparent
pxo replace <from> <to> [--region %r]
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
Release builds set the three values with
`-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`. Without
ldflags, they come from the module version and VCS stamps that Go embeds in
the binary, and fall back to `dev`, `none` and `unknown` (see D21).
