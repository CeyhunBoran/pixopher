# Open questions

Questions that are not decided yet. When one is resolved, move the answer to
[decisions.md](decisions.md) and delete it here.

## Format and history

- **Two-character keys**: is automatic switching to `key_width = 2` enough, or
  should we cap palettes at 82 colors and fail? Key width 2 can already be
  read and written, but nothing switches to it yet (`palette_full`). If we
  switch, how are the existing one-character keys renamed?

## Engine

- **Forced header restores and reused palette indices**: `undo --force` and
  `redo --force` restore a whole header snapshot (D31). If another author
  has since added an entry at the same index (A's auto entry undone, then
  B's auto entry lands at that index), B's pixels silently take A's color.
  The result still validates, so D49's check doesn't catch it. Should a
  forced restore merge palettes, or refuse when an index changes meaning?
  Decide with the palette-aware undo item in M1 (roadmap).

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
  candidate before shipping. `sweetie16` (and `default`, which uses its
  colors) ships with credit to GrafxKid in `palettes/README.md`, but its
  lospec page states no license; confirm with the author's own terms.

## Product

- **Telemetry**: none planned. Keep it that way?
