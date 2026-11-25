# Agent workflow

This doc covers the features that make pixopher pleasant for an AI agent:
drawing a lot per call, seeing the result as text or as an image, getting
objective feedback, and narrating the process for the human watching.

## The loop pixopher is designed for

```
plan  →  block in (batch)  →  look / show --grid  →  lint  →  refine (batch)  →  ...  →  export
```

Every step is one command and returns compact output. `--json` everywhere
lets agents parse results reliably.

## Batch scripts

`pxo run [script|-]` applies many ops as **one transaction**, so one
`undo` reverts the whole run (`undo --step` reverts only the last step).

```sh
pxo run -m "draw the cork and neck" <<'EOF'
# comments start with #
rect 6,1 4,2 cork
rect 6,3 4,1 cork_dark
line 5,3 5,5 ink
line 10,3 10,5 ink
EOF
```

Syntax:

- One op per line, using the same words as the CLI without the `pxo` prefix.
- A comment is `#` at the start of a line, or `#` followed by a space or the
  end of the line. `#` followed directly by hex digits (`#ff8800`) is a
  color, so hex colors need no quoting in scripts.
- Quoting follows shell rules (`"..."`, `'...'`) for args with spaces.
- `--- <message>` starts a new **step** inside the same run. Each step gets
  its own caption in the live viewer and its own history entry and rev. All
  of a run's steps share one `tx` id. With `--atomic` (the default), the
  whole run rolls back if any line fails, and nothing is written.
- `@set layer=shine frame=2` changes the target for the following lines.
- Heredoc blocks for grids are allowed inside scripts:

  ```
  grid 4,4 <<
  ..oo..
  .oRRo.
  ..oo..
  >>
  ```

- On error, nothing is written (unless `--continue-on-error`), and the
  output names the line:
  `script_error line 7: unknown_color "inc", did you mean "ink"?`

**Pacing for viewers**: `--pace 40ms` makes the engine emit each op as a
separate live event with a delay, so the human sees strokes appear one by one
even though the agent sent them in one call. It is purely cosmetic and only
affects viewers. The file is written once at the end.

## Grid paint

An agent "draws" a whole area by writing a letter grid. This matches how
people already hand-write sprites in code.

```sh
pxo grid 4,2 --legend "s=skin S=skin_dark" <<'EOF'
....oooooo....
..oossssssoo..
.ossSsssssssso
EOF
```

- `.` is transparent and **does not overwrite** by default. `--opaque` makes
  `.` erase.
- `_` explicitly erases (it always clears to transparent).
- `--legend` maps grid letters to colors for this call only, so an agent can
  use mnemonic letters without changing the palette keys.
- Rows must have equal length; otherwise the command fails with
  `grid_mismatch` and reports the offending row.
- Symmetry applies, so an agent can write only the left half.

## Grid read: letting text-only agents see [done]

`pxo show` prints the canvas as palette keys (`--grid`, the default).
`--rulers` adds coordinates and `--legend` names the keys shown. After
`pxo new s.pxo --size 12x4`, `pxo rect 2,0 8,4 ink --outline` and
`pxo px 4,1 red 7,2 white`, `pxo show --rulers --legend` prints:

```
   0         1
   012345678901
0  ..oooooooo..
1  ..o.r....o..
2  ..o....w.o..
3  ..oooooooo..
legend: . transparent  o ink #1a1c2c  r red #b13e53  w white #f4f4f4
```

- It reads the cel that drawing commands write (D55). `--region %head`
  crops the output [M2], and `--layer` or `--composite` chooses the source
  [M4].
- Canvases with a side over 64 px are cropped to the content bbox
  automatically (`--full` to disable), and output over 128 px wide is
  printed in 128-column tiles. Rulers always show canvas coordinates, so a
  point can be copied from cropped output into the next command.
- `--json` returns `{"ok":true,"origin":[x,y],"rows":[...],"legend":{...}}`
  whatever the other flags (see [cli.md](cli.md#observation-non-mutating)).
- `pxo get 4,1` and `pxo stats` answer targeted questions cheaply.
- `pxo diff --grid` shows only the changed cells (others as spaces), so the
  agent can check exactly what its last batch did [M5].

## Look: letting multimodal agents see

`pxo look [-o .pixopher/look.png]` renders one **preview sheet** PNG and
prints its path:

```
┌──────────────┬──────────────┬──────────────┬──────────┐
│ 8x on dark   │ 8x on light  │ 8x + grid    │ 1x  2x   │
├──────────────┼──────────────┼──────────────┼──────────┤
│ silhouette   │ changes since│ frames strip (if any)   │
│ (alpha→black)│ last look    │                         │
└──────────────┴──────────────┴─────────────────────────┘
```

- The **1x and 2x** views show the agent how the sprite reads at its real
  size, which is where most mistakes show.
- The **silhouette** tests readability.
- **Changes since last look** highlights the changed pixels (the last look's
  rev is stored in `.pixopher/state`).
- `--frames` adds an animation strip. `--variants` adds one tile per variant.
- The sheet stays small (max ~1024 px wide) so it is cheap to put into a
  model's context.
- The MCP `look` tool returns this image inline.

## Lint

See [lint.md](lint.md). Agents should run `pxo lint` before declaring a
sprite done.

## Narration

- `-m` on any command, and `--- step` in scripts, attach captions.
- Viewers show the caption as a subtitle while the strokes animate in.
- `pxo log` lists the narration. `pxo replay` and `export --timelapse` turn
  it into a shareable recording of the process.
- `--as <name>` labels the author, so multiple agents show as different
  colors.

## Agent guide

`pxo guide` prints a compact manual written for agents (~2–4k tokens):

1. The command cheat-sheet, generated from the op registry.
2. Coordinate and color syntax.
3. The recommended loop (above).
4. **Pixel art craft rules** that the tool can check: pick a light direction
   first, use ramps with hue shifting, keep 1-px lines with consistent step
   patterns (1-1, 2-2, 2-1-2...), avoid orphan pixels, avoid pillow shading,
   outline the silhouette, test readability at 1x, keep colors under the
   palette limit, use clusters rather than noise.

`pxo guide <topic>` (`shading`, `animation`, `tiles`, `palettes`,
`scripts`) prints deeper sections.

`pxo guide --install <dir>` writes the guide as a markdown file
(`pixopher.md`) into a project, so an agent's instruction file can
reference it.

## MCP server

`pxo mcp [--root <dir>]` runs a Model Context Protocol server over stdio, so
agent applications can call pixopher as native tools. `--root` restricts which
files may be opened or written (default: the current directory).

Keep the tool list **small**. Many tiny tools waste the model's context, so
the batch script language does the heavy lifting:

| Tool | Input | Output |
| --- | --- | --- |
| `pixopher_new` | file, size, palette | info |
| `pixopher_run` | file, script, message | result JSON (rev, changed, bbox, warnings) |
| `pixopher_show` | file, region?, layer?, frame? | grid text + legend |
| `pixopher_look` | file, options | **image** content (the preview sheet) + short text |
| `pixopher_lint` | file, rules? | findings JSON |
| `pixopher_info` | file | doc info |
| `pixopher_history` | file, action (log/undo/redo/checkpoint/restore), n? | result |
| `pixopher_export` | file, out, options | written paths |

Resources: `pixopher://guide` (the agent guide), `pixopher://schema` (op
specs), `pixopher://palettes` (library list).

The MCP server uses the same engine as the CLI. If `serve` is running, it
forwards ops there too, so viewers update live during MCP sessions.

## Ideas [idea]

- **Reference-guided drawing**: `import ref.png --as-layer ref --locked
  --opacity 64`, then `lint --compare ref` reports where the drawing differs
  from the reference silhouette.
- **Style presets**: `pxo new --style "1bit"`, `--style "16x16-icon"` set the
  palette, outline mode, light and lint limits in one go.
- **Critique mode**: `pxo look --critique` adds lint markers drawn onto the
  sheet (red boxes at findings), so a multimodal agent sees exactly where.
- **Sprite templates**: `pxo new --template humanoid-32` creates regions,
  anchors and a proportion guide layer. This is not finished art, just a
  scaffold.
