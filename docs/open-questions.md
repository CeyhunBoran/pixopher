# Open questions

Questions that are not decided yet. When one is resolved, move the answer to
[decisions.md](decisions.md) and delete it here.

## Format and history

- **Two-character keys**: is automatic switching to `key_width = 2` enough, or
  should we cap palettes at 82 colors and fail? Key width 2 can already be
  read and written, but nothing switches to it yet (`palette_full`). If we
  switch, how are the existing one-character keys renamed?

## Engine

- **Log and document out of step**: the document is written before the log
  (D32), so a crash in between leaves the log behind the document's rev,
  and a hand edit or `git checkout` can leave it ahead. What should `undo`
  and `log` do with entries above the document's rev, or with a gap? A rev
  rollback also makes the next transaction reuse a tx id (D32), so undo
  can't group entries by tx id alone. Decoded deltas must also be checked
  against the current document (cel, canvas, palette) before they are
  applied. Decide with `undo` in M1.

- **Observation op output**: `Op.Apply` only records changes. `info`,
  `show`, `get` and `stats` need a way to return a result with both a human
  and a `--json` form (a result value on `Ctx`, or a separate
  `Observe(doc, args) (any, error)` interface). Until it is decided, the CLI
  generator rejects ops with `Mutates: false` (D37). Decide with `info`.
- **Repeated argument groups**: `px <x,y> <color> [<x,y> <color> ...]`
  repeats a pair, and `polyline`/`poly` repeat points followed by one color.
  `ArgSpec.Repeat` only lets the last positional repeat. Extend the spec
  (a trailing group of repeating positionals, or a repeat followed by fixed
  ones) when `px` lands.
- **`new` and file resolution**: `new <file>` takes a path that doesn't
  exist yet, so it must skip resolution and the leading-`.pxo` rule. It
  also creates rather than applies. Is that a spec marker (`NoDoc`,
  `Creates`) or a hand-written command? Decide with `new`.
- **Positional strings starting with `-`**: a string or color positional
  such as `-x` is read as a flag unless it follows `--`. `ops.Format`
  doesn't write a `--` for it. No op takes such a value yet.

- **Windows**: are locking and atomic rename reliable enough? Should the
  Windows server use named pipes or localhost TCP + token?
- **Performance budget for 1024x1024**: is parse+serialize per command
  acceptable in direct mode (RLE helps), or should large docs recommend
  `serve`?

## Rendering

- **WebP encoder**: which pure Go lossless encoder is maintained and
  MIT-compatible? Verify before M9.
- **kitty graphics inside tmux**: is passthrough detection reliable enough, or
  should tmux sessions default to halfblock?
- **Caption font for timelapse**: ship a small free bitmap font. Which one,
  and what license?

## Art features

- **Shading algorithm**: is a distance-field pseudo-normal good enough, or do
  we need per-region light overrides (e.g. a hat casting a shadow)? Prototype
  in M6 and compare against hand-shaded fixtures.
- **Lint thresholds**: ΔE limits for `near-duplicate-color` and
  `low-contrast-ramp`, and the step-pattern tolerance for `jaggy-line`. Tune
  them with real sprites.
- **Built-in palettes**: confirm the redistribution license for every
  candidate before shipping.

## Product

- **Telemetry**: none planned. Keep it that way?
