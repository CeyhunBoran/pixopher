# Vision

## One line

A pixel art tool that AI agents can drive fluently from the command line, while
a human watches the piece take shape in real time.

## The problem

Agents can write code, but drawing pixel art with today's tools is painful:

- GUI editors cannot be driven by an agent.
- Generating sprites from hand-written scripts works (a letter grid per sprite
  plus helpers for mirroring, outlining, shading and recoloring), but every
  project ends up reinventing the same helpers, and nobody can watch the
  result until the script runs.
- One-call-per-pixel CLIs are slow for agents, give no feedback on whether
  the art is any good, and lose state when the process dies.
- Agents are often "blind". A text-only agent cannot see a PNG, so it needs a
  textual view of its canvas that it can reason about.

## Goals

1. **Agent-native.** Every feature is usable non-interactively, has
   machine-readable output (`--json`), stable error codes, and helpful hints.
   An agent can draw a whole part in one call (batch scripts, grid paint).
2. **Watchable.** A human runs `pixopher watch` in another pane, or opens the
   web viewer, and sees each stroke appear with a caption saying what the agent
   is doing. Drawing sessions can be replayed and exported as timelapses.
3. **Good art by default.** Palette ramps, auto outline, light-direction
   shading, symmetry and lint give agents the tools pixel artists rely on, so
   results look consistent instead of random.
4. **Durable and diffable.** A drawing is a readable text file (`.pxo`) that
   survives crashes, works in git, and can be edited by hand.
5. **Ship real assets.** Export PNG (integer upscale), SVG, GIF, spritesheets
   with JSON atlases, JPEG, WebP and ICO, including recolor variants in one
   command.
6. **Pure Go, single binary.** No CGO, no GUI libraries. It works over SSH, in
   containers and in CI.

## Non-goals

- A general-purpose raster editor. No brushes with soft edges, no
  anti-aliased painting, no photo editing.
- A native desktop GUI window. The terminal and browser viewers cover live
  viewing without GUI dependencies.
- Canvases larger than 1024x1024. Above that it is not pixel art any more.
- AI image generation inside pixopher. pixopher is the hands and eyes. The
  agent is the artist.

## Who uses it

- **The agent**: draws via CLI, batch scripts or MCP, inspects with
  `show --grid`, `look` and `lint`, and exports assets into a project.
- **The human director**: watches live, reads captions, scrubs the history,
  undoes, and asks for changes. Sometimes they hand-edit the `.pxo` file.
- **The game/app developer**: runs `pixopher export` in a build step to
  produce spritesheets and variants from committed `.pxo` sources.

## What a session looks like

```sh
# pane 1 (human)
pixopher watch hero.pxo

# pane 2 (agent)
pxo new hero.pxo --size 32x32 --palette none
pxo ramp gen skin '#e0a070' --steps 3 --keys "S s t" -m "skin ramp"
pxo palette add o '#1a1c2c' ink
pxo run hero.pxo -m "block in silhouette" <<'EOF'
sym x --axis-x 16
ellipse 16,12 7,8 skin
rect 12,20 8,9 skin
EOF
pxo outline --color ink -m "outline the body"
pxo shade --light top-left -m "first shading pass"
pxo lint
pxo look            # writes a preview sheet for a multimodal agent
pxo export hero.png --scale 4
```

## Prior art and how pixopher differs

[pxcli](https://github.com/vossenwout/pixel-art-cli) by vossenwout is a Go CLI
for agents to draw pixel art. It proved the idea: an agent drawing through
simple shell commands works. It runs an in-memory daemon over a Unix socket,
offers `set_pixel`, `fill_rect`, `line`, `clear`, `get_pixel`, `undo`/`redo`
and PNG export, and can show an optional native window.

pixopher builds on the same idea with a different set of trade-offs:

| | pxcli | pixopher |
| --- | --- | --- |
| State | In-memory daemon, lost on stop | `.pxo` text file as source of truth, optional daemon for speed and live push |
| Calls | One process per primitive | Batch scripts, grid paint, MCP, plus single commands |
| Colors | Free hex / named colors | Palette-first with ramps, hex still allowed |
| Structure | Single bitmap | Layers, frames, tags, regions, anchors, parts |
| Feedback for agents | `get_pixel` | `show --grid`, `look` preview sheets, `lint`, `diff` |
| Live view | Native window (X11/GL) | Terminal viewer and web viewer with captions, replay, timelapse |
| Style helpers | none | Symmetry, outline, light-based shading, recolor variants |
| Export | PNG | PNG, SVG, GIF, spritesheet + JSON, JPEG, WebP, ICO |

Aseprite is the reference for pixel art editor concepts (layers, frames, cels,
tags, palettes). pixopher borrows its vocabulary where it fits.
