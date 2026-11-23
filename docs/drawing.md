# Drawing features

This doc specifies the behavior of the drawing ops. The command syntax is in
[cli.md](cli.md).

## Primitives

| Op | Behavior notes |
| --- | --- |
| `px` | Sets one or more pixels. Out of bounds is an error (unlike shapes). |
| `line` | Bresenham. **Pixel-perfect mode** (default) removes the "L-corner" doubles that make 1-px lines look thick. `--raw` disables it. `--width n` stamps a square brush. |
| `polyline` | Joined lines with shared endpoints drawn once. Pixel-perfect is applied across joints. |
| `rect` | Filled by default. `--outline` draws a 1-px border. |
| `ellipse` / `circle` | Midpoint algorithm, symmetric for both odd and even sizes. Center may be `.5` (`16.5,12`) to get even-width shapes. |
| `poly` | Filled polygon, even-odd rule, pixel-center sampling. Vertices may be fractional. |
| `fill` | 4-connected flood fill by default (`--8` for 8-connected). `--sample composite` decides the boundary from all visible layers but paints on the target layer. |
| `gradient` | Fills a rect by walking a ramp along a direction. With `--dither`, the boundaries between steps use an ordered dither. |
| `dither` | Ordered dither of two colors at a level, for textures and transitions. |
| `grid` | Paints a letter grid at an offset. See [agent-workflow.md](agent-workflow.md#grid-paint). |

All shapes clip to the canvas and respect `--mask`, layer lock and symmetry.

## Symmetry

`pxo sym x --axis-x 16` turns on mirroring for the document:

- Every mutating draw op is applied, then mirrored across the axis. The
  mirrored half is part of the same transaction (one undo step).
- An axis on a `.0` value mirrors pixel *edges* (even-width sprites). An axis
  on `.5` mirrors around a center column (odd-width sprites).
- `xy` mirrors into all four quadrants.
- Ops that already carry their own geometry (`stamp` with `--flip`, `mirror`,
  transforms) are not mirrored again.
- `--no-sym` skips mirroring for one command, which is useful for asymmetric
  details like a scar or a held item.
- **Asymmetric shading**: when lighting comes from one side, the mirrored half
  must not be shaded like the original. `shade` ignores symmetry.

`pxo mirror h --from left` is the one-shot version. It copies the left half
onto the right half once.

## Parts (stamps)

A part is a small named grid stored in the document (`[parts.<name>]`) that
uses the document palette.

- `part save eye %eye_l` captures the pixels from a region.
- `part add bubble <<EOF ... EOF` defines a part from a grid.
- `stamp eye @eye_r --flip h` pastes it. Transparent pixels in a part don't
  overwrite (use `--opaque` to make them clear the target).
- Alignment: `--align tl` (default) puts the part's top-left at the point.
  `c` centers it, and `bc` puts its bottom-center at the point (good for
  standing objects).
- Parts compose: an agent builds a face from `eye`, `mouth_smile`, `brow`,
  then swaps `mouth_smile` for `mouth_o` on the next frame.
- `[idea]` Parts with color slots: a part can reference *ramp steps* instead
  of fixed keys (`$1`, `$2`, `$3`), which are bound at stamp time
  (`--slots skin`). One "hand" part then works for every character.

## Regions and anchors

Regions are named rectangles. Anchors are named points. They exist so agents
can **refer to anatomy by name instead of recomputing coordinates** on every
call.

- `region set head 8,2 16,14`, `anchor set eye_l 12,8`.
- References are `%head`, `@eye_l`, `@eye_l+1,0`, `%head.center`,
  `%head.tl`/`tr`/`bl`/`br`/`top`/`bottom`/`left`/`right`.
- Anchors can be overridden per frame. During animation, `@hand_r` moves, and
  `stamp sword @hand_r` follows it.
- Regions are also masks: `shade --mask %head`, `lint --region %head`.
- Viewers can show regions and anchors as overlays (toggle in `watch` and the
  web viewer).
- `transform`-style ops that move pixels (`shift`, `flip` of a layer) can move
  the regions/anchors inside the affected area with `--move-markers`.

## Outline

`pxo outline [--layer L] [--color ink|auto] [--outer|--inner] [--corners] [--selective]`

- `--outer` (default) adds a 1-px line on transparent pixels next to the
  layer's content. `--inner` recolors the content's border pixels instead
  (the size stays the same).
- `--corners` includes diagonal neighbors (an 8-connected outline). Without
  it, the outline is 4-connected, which reads cleaner at small sizes.
- `--selective` ("selout"): instead of one ink color, each outline pixel
  takes the darkest step of the ramp of the pixel it borders. On the lit
  side (see `--light`), it takes one step lighter. This is the classic look of
  polished sprites.
- `--to-layer L2` writes the outline into a separate layer, so redrawing the
  body and re-outlining stays non-destructive.
- Outlining a *stack* of layers (`--each`) outlines each layer before
  compositing, so overlapping parts keep an ink line between them.

## Shading

`pxo shade [--light top-left] [--layer L] [--strength 1] [--style soft|hard|rim]`

A deterministic, explainable algorithm, not magic:

1. For each opaque pixel of the target layer, compute a pseudo-normal from
   the layer's alpha shape: distance to the edge plus the gradient of a
   distance field (a "pillow-free" bevel).
2. Dot it with the light direction to get a lighting value per pixel.
3. Quantize that value into ramp-step offsets (`-2..+2` × `--strength`).
4. Move each pixel along **its own color's ramp** by that offset. Colors
   that are not in a ramp are left alone (`--ramp-only` is the default).
5. `--style hard` uses fewer bands with clean borders. `soft` adds a
   dithered transition band. `rim` only brightens the lit edge and darkens
   the far edge (good on flat, cel-shaded styles).

The light direction is also stored in the document (`[meta] light =
"top-left"`) so `outline --selective`, `highlight` and `lint` agree on it.

`pxo highlight` adds single-pixel specular highlights on the most lit edge
pixels using the lightest ramp step.

Lint catches "pillow shading" (shading that follows the outline evenly on all
sides), so agents learn not to fake shading by concentric rings.

## Transforms

`flip`, `rotate` (90° steps only, so pixels stay exact), `shift` (with
optional `--wrap` for tiles), `shear` (lean by whole pixels per step), and
`scale` (integer nearest-neighbor). They apply to the target layer, a region
(`--region`), or the whole document (`--all`, all layers and frames).

Arbitrary rotation is **[idea]** (RotSprite-style). It is rarely worth it at
small sizes.

## Recolor and variants

See [palettes.md](palettes.md#variants).

## Animation helpers [planned, M10]

- `frame add --dup` copies the current frame. `frame shift-layer body 0,-1
  --frames 2-3` creates a bob.
- `tween <layer> --from 0 --to 4 --move 0,-3 [--ease in-out]` interpolates a
  layer's offset across frames, in whole pixels.
- Presets: `anim breathe --layer body --frames 4` (1-px squash and stretch),
  `anim bob`, `anim blink --part eye_closed --at @eye_l`.
- Onion skin in viewers (previous/next frame tinted at low alpha).

## Tile mode [planned, M10]

- `pxo new tile.pxo --size 16x16 --tile` sets `[meta] tile = true`.
- In tile mode, shapes and fills wrap around the edges.
- Viewers render a 3x3 repeat so seams are visible.
- `lint seam` checks whether edges match.
- `pxo tileset autotile <terrain-layer> --template blob47|wang16` generates
  the standard autotile sheet from a few hand-drawn corner and edge pieces.
