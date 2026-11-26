# The `.pxo` file format

Status: the document format is **[done]** (M1). The history log is
**[wip]**: entries are written and read, and `undo`, `redo` and `log`
work on them; checkpoints and `log --compact` are still planned.

A `.pxo` file is a **TOML document** whose pixel data is stored as **letter
grids** inside multi-line literal strings. It is meant to be readable, editable
by hand, and to produce useful git diffs.

History lives in a sidecar file, `<name>.pxo.log`, so the main file stays
clean (see [History log](#history-log-wip)).

## Example

This example is hand-written. It spells out the defaults of the first layer
so every field is visible. The canonical writer omits defaults such as
`visible = true` (see
[Canonical serialization](#canonical-serialization)).

```toml
pixopher = 1                 # format version
size = [16, 16]              # width, height (max 1024 x 1024)
rev = 12                     # bumped on every history step
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
skin   = { steps = ["r", "R", "w"], base = "w" }   # only when the base isn't the middle step

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
- `key_width`: 1 by default, or 2 for palettes that need more than 82 keys
  (see below).

### Palette keys

- With `key_width = 1`, a key is one printable ASCII character. These are
  **reserved** and can never be keys:
  - space, `"`, `'`, `\` and `#`: TOML, shell and comment syntax.
  - `@`, `%`, `:`, `+`, `-` and `,`: reference and color syntax (`@anchor`,
    `%region`, `ramp:1`, `ramp+1`, `x,y`).
  - `.`: always transparent.
  - `_`: the erase marker in grid paint. It means "clear to transparent" and
    is never stored in a cel.

  That leaves 82 keys (`A-Z a-z 0-9` plus 20 punctuation characters).
- Bigger palettes use `key_width = 2` (keys like `a1`, `Zz`). An existing
  document never switches on its own: `palette widen` [M9] rewrites it,
  doubling every key (`o` → `oo`). A command that creates a document with a
  bigger palette may start at width 2 (D56). Then `..` is transparent and `__`
  erases, and keys must not contain reserved characters. This is rare for
  pixel art.
- An entry is `key = [color, name]`, and both are required. Colors are
  `#rrggbb` or `#rrggbbaa`. Names follow the rules in
  [decisions.md](decisions.md) (D27).
- `.` is always `["#00000000", "transparent"]`. Writers put it first, and
  readers add it if it is missing.
- Drawing with a raw hex color that is not in the palette adds an **auto
  entry**: a generated key with a name like `auto_3f2a1b`. Grids therefore
  only ever contain keys. `lint` warns about auto entries
  (`off-palette`), and `palette tidy` merges or names them.
  When no key is free, the command fails with `palette_full`. Documents are
  never widened automatically (D56).

### Ramps

A ramp is an array of palette keys, dark to light. Its base step (used by the
bare ramp name and by `skin+1`) defaults to the middle step, `(n - 1) / 2`.
A different base is stored with an inline table:
`skin = { steps = ["S", "s", "t"], base = "s" }`.

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
  In a token, the key is the last `key_width` characters and the count is
  the rest, so digit keys are unambiguous (`121` is 12 × `1`).
- Readers reject `_` in stored grids, ragged rows and grids outside the
  canvas (`grid_mismatch`).

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

### Unknown keys

Readers reject keys they don't know with `parse_error`, so a rewrite never
silently drops data (D25).

### Canonical serialization

Writers always output sections in the order above, with `[variants.*]` and
`[lint]` after `[anchors]`, and `[[cels]]` last. They sort name-keyed tables
(except the palette, whose order is preserved because it is meaningful), omit
defaults and empty sections, use LF line endings, and end the file with a
newline. `meta.created` is written in UTC and must have an offset when read.
This keeps diffs minimal and makes golden tests stable. The exact rules are in
D24 in [decisions.md](decisions.md).

## History log [wip]

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
  since changed the same pixels or palette entries, undo fails with
  `undo_conflict` unless `--force` is given (see D20 and D54 in
  [decisions.md](decisions.md)).

- `ops` holds the normalized script lines that were applied. This powers
  replay, timelapse and the captions in viewers.
- `undo` holds the deltas in both directions, enough to undo and redo
  without document snapshots **[done]**. `undo.pixels` is base64 of
  run-packed pixel deltas, `""` when no pixel changed. `undo.meta` is `[]`,
  or one `{"before", "after"}` pair with the canonical text of the document
  header (no cels, `rev = 0`) when the palette, ramps or another non-pixel
  part changed. The exact encoding is D31 in [decisions.md](decisions.md).
- `msg` is omitted when the step has no caption, and `bbox` when no pixel
  changed. A normal step that changes nothing writes no entry (D32);
  undo and redo entries are always written (D50).
- Every 100 revisions a **checkpoint** entry embeds a full snapshot of the
  document, so replay and `restore` don't need to walk from rev 0.
- Named checkpoints are created with `pixopher checkpoint "before arms"`.
- `undo` and `redo` append entries too **[done]**: one per reverted or
  re-applied entry, with `"kind": "undo"` or `"redo"` after `author`,
  `"target"` (the rev of the entry acted on), `"ops": []`, and their own
  deltas in `undo`. Normal entries have no `kind`. The log is never
  rewritten, so the drawing process can always be replayed, mistakes
  included (D50):

  ```json
  {"rev":13,"tx":"t_0013","ts":"2026-11-23T18:05:00Z","author":"agent","kind":"undo","target":12,"ops":[],"bbox":[6,1,4,3],"undo":{"pixels":"<base64 packed deltas>","meta":[]}}
  ```

- **Effective history** **[done]**: readers follow the chain that leads to
  the document's rev. Reading backward from the end, an entry counts only
  when its rev is below the last one counted, starting below the
  document's rev + 1. Entries above the document's rev and entries
  abandoned after a rollback are ignored, and undo checks every entry
  against the document before applying it (D49).
- `pixopher log --compact` rewrites the log, keeping checkpoints and the
  messages.

Commit the log to git next to its document; it powers replay and timelapse.
To keep diffs readable, add `*.pxo.log -diff` to `.gitattributes`,
and run `log --compact` if a log grows too
large. If the log is missing, the document still opens, with no undo history.
If its last line is incomplete (a crash during append), readers ignore that
line and the next append cuts it off.

Writers also create `<name>.pxo.lock`, the advisory lock file. It is
never deleted and holds no data; add `*.pxo.lock` to `.gitignore`.

## Versioning

- Additive changes (new optional keys) keep `pixopher = 1`.
- Breaking changes bump the major version, and `pixopher migrate` upgrades
  old files.
