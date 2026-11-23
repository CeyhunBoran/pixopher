# Live view

The human watches while the agent draws. There are two viewers. Both are
read-only by default, and neither needs a GUI toolkit.

## Event model

Viewers consume a stream of **events**:

```json
{"type":"tx","rev":13,"author":"agent","msg":"draw the cork","ops":["rect 6,1 4,2 cork"],"bbox":[6,1,4,2],"step":1,"steps":3}
{"type":"op","rev":13,"op":"rect 6,1 4,2 cork","pixels":[[6,1,"C"],...]}     // only with --pace or from serve
{"type":"reload","rev":14}                                                    // external edit, full re-read
```

- **Direct mode**: viewers watch `sprite.pxo` and `sprite.pxo.log` with
  `fsnotify`. On change, they re-read the file and tail the log for the
  newest `tx` event.
- **Server mode**: viewers subscribe to the daemon (SSE over HTTP, or the
  socket). They get `op` events in real time, including paced replays of
  batches.

## `pxo watch` (terminal viewer) [M3]

```
┌ hero.pxo  32x32  rev 13  layer body  frame 0/3 ──────────────────────────┐
│                                         │ palette                        │
│        ▄▄████▄▄                         │ o ink        ██                │
│      ▄██▀▀▀▀▀▀██▄                       │ s skin       ██  ramp skin 1/3 │
│      ██  ▄▄  ▄▄██      (canvas)         │ S skin_dark  ██                │
│      ▀██▄▄▄▄▄▄██▀                       │ layers                         │
│                                         │ ▸ body  ◉   shine ◉   ref ○    │
├─────────────────────────────────────────┴────────────────────────────────┤
│ agent ▸ step 2/3: "first shading pass"                    +86px  12ms ago│
└──────────────────────────────────────────────────────────────────────────┘
```

### Rendering modes (`--mode`, default `auto`)

| Mode | How | When |
| --- | --- | --- |
| `kitty` | kitty graphics protocol, real pixels, crisp at any zoom | kitty, WezTerm, Ghostty, Konsole (detected via query) |
| `sixel` | sixel images | xterm with sixel, foot, mlterm, some others |
| `halfblock` | `▀` with truecolor fg/bg, giving 2 pixels per cell vertically | any truecolor terminal (alacritty, gnome-terminal, ...) |
| `ascii` | palette keys as characters, like `show --grid` | no color, logging, accessibility |

Transparent pixels are drawn on a checkerboard (toggle `b` for a dark, light
or checker background).

### Features

- Auto-fit zoom, with `+`/`-` for integer zoom and arrow keys or `hjkl` to
  pan. A 1024x1024 canvas is shown with a viewport and a minimap.
- **Stroke flash**: pixels changed by the latest op briefly highlight (an
  inverted or bright border), so you see where the agent is drawing.
- **Caption bar**: the current author, step and message, from the history log.
- Toggles: `g` pixel grid (kitty/sixel only), `r` regions/anchors overlay,
  `o` onion skin, `s` symmetry axis, `L` layer solo cycling, `space`
  play/pause animation (per-frame durations, tags), `t` tile 3x3 preview.
- `u`/`U` undo/redo from the viewer. This is the only write action, it is
  disabled with `--read-only`, and it goes through the normal engine.
- Works over SSH and in tmux (halfblock is always safe; kitty in tmux needs
  passthrough and is detected).

## `pxo serve` web viewer [M7]

`pxo serve [files...] [--port 7777] [--open]` serves `http://127.0.0.1:7777`.

- Static assets are embedded (`embed.FS`): plain HTML, CSS and JS with a
  `<canvas>`, no framework and no build step.
- **Live canvas** with `image-rendering: pixelated`, zoom, pan, pixel grid,
  checkerboard, and region/anchor overlays.
- **Timeline scrubber** over the history log: drag to any rev, step through
  ops, and read each caption. Scrubbing is a view only; "restore to here"
  is an explicit button.
- **Animation player** for frames and tags, with onion skin.
- **Side panel**: palette, ramps, layers, frames, lint findings (click to
  jump to a pixel).
- **Record**: download the session as GIF or WebM, recorded client-side from
  the canvas, or trigger `export --timelapse` on the server.
- **Multiple files**: tabs for each served document, which helps when an
  agent draws a whole icon set.
- Security: bind to `127.0.0.1` only. A random token in the URL is required
  for write actions. The token is printed on start.

## Replay and timelapse [M7]

- `pxo replay [--from rev] [--to rev] [--speed 2x] [--tui]` re-plays the
  history log in `watch` (or streams it to an open web viewer).
- `pxo export --timelapse out.gif [--fps 12] [--scale 8] [--captions]
  [--hold-last 2s]` renders one frame per op, or per tx with
  `--per-step`. It can burn captions in with a small built-in bitmap font,
  and holds the final image at the end.
- `--per-pixel` animates each pixel of each op for a "being drawn" feel.
  Long sessions are sped up automatically to a target duration
  (`--duration 20s`).
