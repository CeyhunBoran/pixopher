# Export and import

## Export

```
pxo export <out> [options]
```

The format comes from the output extension, or from `--format`. The output
path may contain placeholders: `{name}` (doc file name), `{frame}`, `{tag}`,
`{layer}`, `{variant}`, `{scale}`. For example,
`out/{name}-{variant}@{scale}x.png`.

### Common options

| Option | Meaning |
| --- | --- |
| `--scale n` | integer nearest-neighbor upscale (1–64). Also `--size 128x128` if it is an exact integer multiple. |
| `--frame n` / `--tag name` / `--all-frames` | which frames |
| `--layer L` / `--composite` (default) / `--each-layer` | which layers; hidden layers are skipped unless `--include-hidden` |
| `--variant v` / `--variants all` | recolor variants (see [palettes.md](palettes.md#variants)) |
| `--bg color` | flatten onto a background |
| `--crop auto` | trim transparent borders |
| `--padding n` | transparent margin (in art pixels, before scaling) |

### Formats

| Format | Milestone | Notes |
| --- | --- | --- |
| **PNG** | M1 | Lossless and the default. Writes indexed PNG (palette chunk + tRNS) when ≤256 colors, which gives small files, and RGBA otherwise. `--rgba` forces RGBA. |
| **SVG** | M4 | One `<rect>` per merged run. A greedy rectangle merge per color minimizes elements. `shape-rendering="crispEdges"`, `viewBox` in art pixels, and `width`/`height` from `--scale`. `--css-vars` emits palette colors as CSS custom properties, so the web can recolor at runtime. |
| **GIF** | M4 | Animated from frames/tags, using per-frame durations (GIF uses 10 ms units, so durations get rounded and a warning is printed). Transparency uses one reserved index. `--loop n` (0 = forever). |
| **Spritesheet** | M4 | `--sheet grid --cols 8` (or `row`, `col`, `packed`) PNG, plus `--atlas out.json` with frame rects, durations, tags, and anchors per frame. The atlas format is pixopher's own simple JSON. `--atlas-format aseprite` writes an Aseprite-compatible JSON array/hash for engines that already read it. |
| **JPEG** | M9 | Lossy. It **always flattens** onto `--bg` (default white) and **always prints a warning**: "JPEG blurs pixel art; prefer PNG/WebP". `--quality` defaults to 95, and chroma subsampling is disabled when the encoder allows. It is mostly useful for previews and social posts at a large `--scale`. |
| **WebP** | M9 | **Lossless** by default (pure Go encoder). `--lossy --quality` is optional and comes with the same warning as JPEG. Animated WebP is **[idea]**. |
| **ICO** | M9 | Multi-size favicon/app icon. `--sizes 16,32,48,256` renders each size by integer scale from the source when it divides evenly. Otherwise the export fails with a hint, or uses `--resample nearest` (with a warning). The images are stored PNG-compressed inside the ICO. |
| **JSON** | M4 | `export doc.json`: a resolved, flattened RGBA dump (`{w,h,frames:[[["#rrggbbaa",...]]]}`) for tooling. |

Every export prints the written paths (or JSON `{"written":[...]}`).

### Build pipelines

`pxo export` is designed to run in a project's build step:

```sh
pxo export sprites/*.pxo --out 'assets/{name}.png' --scale 4
pxo export icons/app.pxo app.ico --sizes 16,32,48,256
```

Multiple inputs are allowed with `--out` templates. Exports are
deterministic (byte-identical across runs and platforms), so generated assets
don't cause noisy diffs.

## Import [M9]

```
pxo import <in.(png|gif|jpg|webp)> [new.pxo] [options]
```

| Option | Meaning |
| --- | --- |
| `--palette keep` | every distinct color becomes a palette entry (fails if > limit) |
| `--palette quantize:16` | reduce to N colors (median cut in OKLab, then k-means refinement) |
| `--palette doc` / `--palette <name|file>` | map every pixel to the nearest color of an existing palette |
| `--pixelate 32x32` | downscale a large image to pixel art size first (mode/median per block, not averaging, so edges stay crisp) |
| `--detect-scale` | detect an upscaled pixel art image (e.g. 4x) and import it at 1x |
| `--as-layer <name>` | import into an existing doc as a new layer |
| `--locked --opacity 64` | typical for a **reference layer** to trace over; reference layers are excluded from export by default |
| `--frames` | import GIF frames or a spritesheet (`--sheet 8x1`) into frames |
| `--ramps auto` | guess ramps by clustering palette colors by hue and sorting by lightness |

Palette file import (`.hex .gpl .pal .ase`) lives under `palette load`.

`[idea]` Import `.aseprite` files (layers, frames, tags, palette) for people
coming from that editor. Export to `.aseprite` is lower priority.
