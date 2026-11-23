# Palettes and color

pixopher is **palette-first**. Every document has a palette of named entries,
and entries can be grouped into **ramps**: ordered shades from dark to light.
Raw hex colors are allowed but discouraged (see
[file-format.md](file-format.md#palette-keys)).

There are three reasons for this design:

1. **Consistent style.** Agents that pick random hex values produce muddy
   art. A small palette forces coherence.
2. **Smart operations.** Shading, outlining (selective outline), recoloring
   and variants all work by moving along or swapping ramps.
3. **Readable grids.** One key per color makes `show --grid` and the `.pxo`
   format readable.

## Color syntax (everywhere a color is accepted)

| Form | Example | Meaning |
| --- | --- | --- |
| Palette key | `o` | entry with key `o` |
| Palette name | `ink` | entry named `ink` |
| Ramp name | `skin` | the ramp's base step (`--base`, default: middle step) |
| Ramp step | `skin:0`, `skin:-1` | ramp `skin`, index from dark (0) or from light (-1) |
| Ramp relative | `skin+1`, `skin-1` | one step lighter/darker than the ramp's *base* step |
| Hex | `#f80`, `#ff8800`, `#ff880080` | raw color (creates an auto palette entry) |
| Transparent | `.`, `transparent`, `none` | alpha 0 |

Ambiguity rule: a single character is a key first. A word is a palette
entry name first, then a ramp name.
`key:` and `name:` prefixes (`key:o`, `name:o`) force one interpretation.
Errors suggest the closest names (`unknown color "inc", did you mean "ink"?`).

Shell note: `#` starts a comment in shells, so quote hex colors in shell
commands (`'#ff8800'`). Batch scripts don't need quotes.

## Palette commands

```
pxo palette list                       # keys, names, hex, ramps; --json
pxo palette add k '#ffcc99' skin       # key, color, name
pxo palette set skin '#ffd0a0'         # change a color (all pixels using it change)
pxo palette rename skin flesh
pxo palette rm k [--replace o]         # remove; pixels using it are remapped or the command fails
pxo palette load sweetie16             # built-in or file (.hex .gpl .pal .ase .png strip)
pxo palette save my.gpl
pxo palette tidy                       # merge duplicates, name auto entries, drop unused (with --unused)
pxo ramp add skin S s t [--base s]
pxo ramp gen skin '#e0a070' --steps 4 --hue-shift 12 --keys "S s t T"
pxo ramp rm skin
```

Writers quote palette keys that are not bare TOML keys. Bare keys are
`A-Za-z0-9_-`. Every other key, `.` included, is quoted.

## Ramp generation [planned]

`ramp gen skin <base> --steps n --keys "..."` creates n palette entries named
`skin_0` (darkest) to `skin_<n-1>`, using the given keys or auto-assigned
ones, plus a ramp `skin` over them. It builds a pixel-art-style ramp from a single base color in OKLCH
space:

- lightness steps spread evenly around the base,
- **hue shifting**: darker steps rotate toward cool or purple, lighter steps
  toward warm or yellow (`--hue-shift` degrees, default 10),
- chroma that drops at the extremes, so shadows and highlights don't get
  over-saturated,
- a minimum ΔE between neighbors (`lint low-contrast-ramp` uses the same
  threshold).

## Variants

A variant is a named set of palette overrides. Pixels are never duplicated.

```
pxo variant add night --ramp glass='#29366f,#3b5dc9,#41a6f6' --set shine='#94b0c2'
pxo variant add winter --swap leaves=snow            # map one ramp onto another ramp of the same length
pxo variant list
pxo export out/potion-{variant}.png --variants all
```

`pxo recolor` is the destructive cousin. It changes the base palette or
remaps pixels:

```
pxo recolor glass=liquid            # every pixel using ramp glass now uses ramp liquid, step by step
pxo recolor R=o --region flask      # remap a single key inside a region
```

## Built-in palette library [planned]

Ship a set of well-known, freely licensed palettes embedded in the binary.
Only palettes whose authors allow redistribution get included, and the
attribution goes in `palettes/README.md`. Candidates:

- `sweetie16`, `endesga32`, `endesga64`, `resurrect64`, `apollo`, `na16`,
  `db16`, `db32`, `pico8`, `gameboy` (4-shade green), `1bit`.
- Each built-in palette ships with **suggested ramps**, so shading works
  right away.

`pxo palette library` lists them with swatches (rendered in the terminal).

Import formats: `.hex` (one hex per line), `.gpl` (GIMP), `.pal` (JASC),
`.ase`/`.aseprite` palette chunk, and a PNG strip (one pixel per color).
