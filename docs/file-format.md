# The `.pxo` file format

A `.pxo` file is a **TOML document** whose pixel data is stored as **letter
grids** inside multi-line literal strings. It is meant to be readable, editable
by hand, and to produce useful git diffs.

History lives in a sidecar file, `<name>.pxo.log`, so the main file stays
clean (see [History log](#history-log)).

## Example

```toml
pixopher = 1                 # format version
size = [16, 16]              # width, height (max 1024 x 1024)
rev = 12                     # bumped on every transaction
key_width = 1                # characters per pixel in grids (1 or 2)

[meta]
title = "potion"
created = 2026-11-23T18:00:00Z

[palette]                    # key = [color, name]
"." = ["#00000000", "transparent"]  # "." is always transparent, reserved
o   = ["#1a1c2c", "ink"]
g   = ["#38b764", "glass"]
G   = ["#a7f070", "glass_light"]
r   = ["#b13e53", "liquid_dark"]
R   = ["#ef7d57", "liquid"]
w   = ["#f4f4f4", "shine"]

[ramps]                      # ordered dark → light, by palette key
glass  = ["o", "g", "G"]
liquid = ["r", "R", "w"]

[symmetry]
mode = "x"                   # off | x | y | xy | (later) radial
axis_x = 8.0                 # may be .5 for odd/even centering

[[layers]]
name = "body"
visible = true
locked = false
opacity = 255
blend = "normal"

[[layers]]
name = "shine"

[[frames]]
duration_ms = 120

[[tags]]                     # animation ranges over frames (0-based, inclusive)
name = "idle"
from = 0
to = 0
direction = "forward"        # forward | reverse | pingpong

[regions]                    # named rectangles: [x, y, w, h]
neck = [6, 1, 4, 3]
flask = [3, 4, 10, 11]

[anchors]                    # named points: [x, y]; per-frame overrides allowed
cork = [8, 1]

[parts.bubble]               # reusable stamp, own grid, uses the doc palette
grid = '''
.w.
w.w
.w.
'''

[[cels]]                     # pixel data for one (layer, frame)
layer = "body"
frame = 0
origin = [0, 0]              # top-left of the stored grid; cels are cropped to content
grid = '''
......oooo......
......oGgo......
......oooo......
.....o....o.....
....o.gGGg.o....
'''
```

## Rules

### Header

- `pixopher` (required): format version. Readers reject unknown major
  versions with `unsupported_version`.
- `size` (required): `[w, h]`, each value from 1 to 1024.
- `rev`: integer, starts at 0.
- `key_width`: 1 by default. Documents switch to 2 automatically when the
  palette outgrows single-character keys (see below).

### Palette keys

- With `key_width = 1`, a key is one printable ASCII character. These are
  **reserved** and can never be keys:
  - space, `"`, `'`, `\` and `#`: TOML, shell and comment syntax.
  - `@`, `%`, `:`, `+`, `-` and `,`: reference and color syntax (`@anchor`,
    `%region`, `ramp:1`, `ramp+1`, `x,y`).
  - `.`: always transparent.
  - `_`: the erase marker in grid paint. It means "clear to transparent" and
    is never stored in a cel.

  That leaves about 85 keys (`A-Z a-z 0-9` plus some punctuation).
- When a palette needs more keys, the document is rewritten with
  `key_width = 2` (keys like `a1`, `Zz`). Then `..` is transparent and `__`
  erases, and keys must not contain reserved characters. This is rare for
  pixel art.
- Colors are `#rrggbb` or `#rrggbbaa`.
- Drawing with a raw hex color that is not in the palette adds an **auto
  entry**: a generated key with a name like `auto_3f2a1b`. Grids therefore
  only ever contain keys. `lint` warns about auto entries
  (`off-palette`), and `palette tidy` merges or names them.

### Cels and grids

- A new document has one layer named `main` and one frame (index 0) with
  `duration_ms = 100`. A document always has at least one layer and one
  frame.
- One cel per (layer, frame) that has content. A missing cel means empty.
- `origin` + the grid's dimensions define the cel's rectangle. Writers crop
  cels to their content bounding box. Readers accept any rectangle inside the
  canvas.
- A grid is a TOML literal multi-line string (`'''`). Each line is one row,
  every row has the same width, and the first newline after `'''` is ignored.
- `link = { frame = 0 }` instead of `grid` means "same pixels as this layer's
  cel in frame 0". This saves space for static layers across animation frames.
- **RLE encoding** for large cels: `encoding = "rle"` and `rows = [...]`,
  where each row is a sequence of `<count><key>` tokens, e.g. `"12. 4o 16."`.
  Writers use RLE automatically when a cel is wider than 128 px or when RLE
  is less than half the size of the raw grid. Readers support both.

### Anchors per frame

`[anchors]` holds default positions. Per-frame overrides live on frames:

```toml
[[frames]]
duration_ms = 100
anchors = { cork = [8, 0] }
```

### Variants

Recolor variants are stored as palette overrides, not as copies of the pixels:

```toml
[variants.night]
glass = ["#29366f", "#3b5dc9", "#41a6f6"]   # ramp name → new colors, same length
shine = "#94b0c2"                            # single palette entry by name
```

`export --variant night` or `--variants all` renders them. See
[palettes.md](palettes.md#variants).

### Lint config

```toml
[lint]
max_colors = 16
disable = ["banding"]
```

### Canonical serialization

Writers always output sections in the order above, sort keys inside tables
(except palette order, which is preserved because it is meaningful), use LF
line endings, and end the file with a newline. This keeps diffs minimal and
makes golden tests stable.

## History log

`<name>.pxo.log` is **JSON Lines**. There is one entry per transaction, and
the file is append-only.

```json
{"rev":12,"tx":"t_0007","ts":"2026-11-23T18:04:11Z","author":"agent","msg":"add cork","ops":["rect 6,1 4,3 ink"],"bbox":[6,1,4,3],"undo":{"pixels":"<base64 packed deltas>","meta":[]}}
```

- **Entries and transactions**: an entry is one *step*, with one rev and one
  caption. A transaction (`tx`) is one command or one `run`. A `run` with
  `--- step` markers writes several entries that share the same `tx` id.
  Single commands write one entry.
- **Undo works on transactions**: `undo` reverts every entry of the latest
  non-undone `tx` by the current author (`--any` for anyone's). `undo --step`
  reverts only the latest entry. `redo` mirrors both. If another author has
  since changed the same pixels, undo fails with `undo_conflict` unless
  `--force` is given (see D20 in [decisions.md](decisions.md)).

- `ops` holds the normalized script lines that were applied. This powers
  replay, timelapse and the captions in viewers.
- `undo` holds the inverse deltas, enough to revert without snapshots.
- Every 100 revisions a **checkpoint** entry embeds a full snapshot of the
  document, so replay and `restore` don't need to walk from rev 0.
- Named checkpoints are created with `pixopher checkpoint "before arms"`.
- `undo` and `redo` append entries too (`{"kind":"undo","target":12}`). The
  log is never rewritten, so the drawing process can always be replayed,
  mistakes included.
- `pixopher log --compact` rewrites the log, keeping checkpoints and the
  messages.

Commit the log to git next to its document; it powers replay and timelapse.
To keep diffs readable, add `*.pxo.log -diff` to `.gitattributes`,
and run `log --compact` if a log grows too
large. If the log is missing, the document still opens, with no undo history.

## Versioning

- Additive changes (new optional keys) keep `pixopher = 1`.
- Breaking changes bump the major version, and `pixopher migrate` upgrades
  old files.
