# Lint [M5]

`pxo lint` checks a document for common pixel art mistakes and reports them
with coordinates. It gives agents an objective feedback loop: draw, lint,
fix.

```
$ pxo lint
warn  orphan-pixel        12,7   single pixel of "skin_dark" surrounded by "skin"
warn  jaggy-line          4,10–9,15  uneven steps 1-2-1-3; try a consistent pattern (1-1 or 2-2)
info  pillow-shading      %head  shading follows the outline on all sides; pick a light direction
error palette-overflow    -      19 colors used, limit is 16
3 findings (1 error, 2 warn)
```

`--json` prints `[{"rule","severity","at":[x,y]|"rect":[x,y,w,h],"message","hint","fix"?}]`.

Flags: `--rule <id>` (repeatable), `--region %r`, `--layer L`,
`--composite` (default), `--frame n|--all-frames`,
`--fail-on error|warn|info` (exit 5), `--fix` (applies only rules marked
fixable).

Config lives in the document's `[lint]` table (`max_colors`, `disable`,
per-rule thresholds).

## Rules

| Rule | Default severity | Checks | Fixable |
| --- | --- | --- | --- |
| `palette-overflow` | error | more colors used than `max_colors` | no |
| `off-palette` | warn | auto entries from raw hex colors | `palette tidy` |
| `near-duplicate-color` | warn | two palette colors with ΔE (OKLab) below threshold | merge |
| `low-contrast-ramp` | warn | neighboring ramp steps too close to tell apart | no |
| `unused-color` | info | palette entries with no pixels | yes (remove) |
| `orphan-pixel` | warn | a single pixel whose 8 neighbors are all one other color (noise) | yes (absorb) |
| `jaggy-line` | warn | 1-px line runs with inconsistent step lengths | no (hint gives a pattern) |
| `doubled-line` | warn | L-shaped corners and 2-px-thick spots in 1-px outlines | yes |
| `outline-gap` | warn | a break in an outline that otherwise encloses the shape | no |
| `banding` | info | parallel color bands that line up stair-step with the edge | no |
| `pillow-shading` | info | shading levels concentric with the silhouette (no light direction) | no |
| `light-inconsistency` | info | highlights on the side facing away from `[meta] light` | no |
| `stray-alpha` | warn | partially transparent pixels (alpha not 0 or 255) unless allowed | yes (snap) |
| `silhouette-noise` | info | the silhouette at 1x has single-pixel protrusions or holes | no |
| `symmetry-break` | info | with symmetry on, pixels that differ between the halves | no |
| `empty-layer` / `empty-frame` | info | nothing drawn | no |
| `seam` (tile mode) | warn | left/right or top/bottom edges don't match | no |
| `frame-jitter` (animation) | info | the silhouette's bbox shifts more than N px between frames without intent | no |

Each rule is a small, separately tested function in `internal/lint`, with a
fixture `.pxo` that triggers it and one that doesn't.

## Design notes

- Lint is **advice, not law**. Pixel art breaks rules on purpose, so
  defaults lean toward `info`/`warn`. Only objective violations (like a
  palette over its limit) are errors.
- Messages always include a **hint** the agent can act on.
- Findings are stable (deterministic order: severity, rule, y, x), so agents
  can diff lint output between iterations.
- `look --critique` draws the findings onto the preview sheet.
