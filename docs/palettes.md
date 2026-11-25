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

## Color syntax (everywhere a color is accepted) [done]

| Form | Example | Meaning |
| --- | --- | --- |
| Palette key | `o` | entry with key `o` |
| Palette name | `ink` | entry named `ink` |
| Ramp name | `skin` | the ramp's base step (`--base`, default: middle step) |
| Ramp step | `skin:0`, `skin:-1` | ramp `skin`, index from dark (0) or from light (-1) |
| Ramp relative | `skin+1`, `skin-1` | one step lighter/darker than the ramp's *base* step |
| Hex | `#f80`, `#ff8800`, `#ff880080` | raw color (creates an auto palette entry) |
| Transparent | `.`, `transparent`, `none` | alpha 0 |

Ambiguity rule: a single character (two with `key_width = 2`) is a key
first. A word is a palette entry name first, then a ramp name.
A hex color that exactly matches an entry resolves to it, and any hex with
alpha 0 is transparent. Ramp steps out of range fail with `invalid_color`,
and unknown names fail with `unknown_color`.
`key:` and `name:` prefixes (`key:o`, `name:o`) force one interpretation.
Errors suggest the closest name in their hint (message `unknown color "inc"`,
hint `did you mean "ink"?`).

Shell note: `#` starts a comment in shells, so quote hex colors in shell
commands (`'#ff8800'`). Batch scripts don't need quotes.

## Palette commands

```
pxo palette list                       # keys, names, hex, ramps; --json   [done]
pxo palette add k '#ffcc99' skin       # key, color, name   [done]
pxo palette set skin '#ffd0a0'         # change a color (all pixels using it change)   [done]
pxo palette rename skin flesh          # [done]
pxo palette rm k [--replace o]         # remove; pixels using it are remapped or the command fails   [done]
pxo palette load endesga32             # built-in or file (.hex .gpl .pal .ase .png strip)
pxo palette save my.gpl
pxo palette tidy                       # merge duplicates, name auto entries, drop unused (with --unused)
pxo ramp add skin S s t [--base s]     # [done]
pxo ramp gen skin '#e0a070' --steps 4 --hue-shift 12 --keys "S s t T"
pxo ramp rm skin                       # [done]
```

Entries are picked by key or name (`key:` and `name:` force one); ramp steps
and hex colors don't pick an entry. `add` and `set` take `#hex` colors.
Removing an entry renumbers the entries after it on every cel and part. It
fails with `in_use` while the entry is a ramp step or a variant override, or
while pixels use it and no `--replace` is given. A taken key or name is
`already_exists` (D53).

`palette list --json` is one line (wrapped here):

```json
{"ok": true, "key_width": 1,
 "entries": [{"key": "o", "name": "ink", "color": "#1a1c2c"}, {"key": "g", "name": "glass", "color": "#38b764"}],
 "ramps": {"glass": {"steps": ["o", "g"], "base": 0}}}
```

Without `--json` it prints one line per entry, then one per ramp with the
base step in brackets:

```
o  #1a1c2c  ink
g  #38b764  glass
ramp glass  [o] g
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

## Built-in palette library [wip]

`default` is **[done]** (D53): the 16 colors of Sweetie 16 by GrafxKid, with
the ramps `warm`, `green`, `blue` and `gray`, and keys that recall the names
(`o` ink, `r` red, `c` coral, `y` yellow, `l` lime, `g` green, `t` teal, `n`
navy, `b` blue, `s` sky, `i` ice, `w` white, `f` fog, `a` ash, `d` dusk, `p`
plum). `pxo new` starts from `default`; `--palette none` gives an empty
palette.

The rest is **[planned]**: ship a set of well-known, freely licensed palettes embedded in the binary.
Only palettes whose authors allow redistribution get included, and the
attribution goes in `palettes/README.md`. Candidates:

- `endesga32`, `endesga64`, `resurrect64`, `apollo`, `na16`,
  `db16`, `db32`, `pico8`, `gameboy` (4-shade green), `1bit`.
- Each built-in palette ships with **suggested ramps**, so shading works
  right away.

`pxo palette library` lists them with swatches (rendered in the terminal).

Import formats: `.hex` (one hex per line), `.gpl` (GIMP), `.pal` (JASC),
`.ase`/`.aseprite` palette chunk, and a PNG strip (one pixel per color).
