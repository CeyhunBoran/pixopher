# pixopher

A pixel art CLI that AI agents can draw with, while you watch the drawing
come together live in your terminal or browser.

> **Status: planning.** Nothing is implemented yet. The design lives in
> [`docs/`](docs/README.md), and the build order is in
> [`docs/roadmap.md`](docs/roadmap.md).

## Planned features

- **Readable `.pxo` documents**: text files with letter-grid pixels that git
  can diff, with layers, frames, tags, regions and anchors.
- **Agent-native**: batch scripts, grid paint and read, `--json` everywhere,
  stable error codes, and an MCP server.
- **Eyes for agents**: `show --grid` for text-only agents, `look` preview
  sheets for multimodal ones, and `lint` for pixel art mistakes.
- **Palette-first**: ramps, hue-shifted ramp generation, recolor variants,
  and a built-in palette library.
- **Style helpers**: symmetry, stamps, selective outline, light-direction
  shading.
- **Live view**: `pixopher watch` (kitty / sixel / half-block) and
  `pixopher serve` (web viewer with timeline), with narrated captions, replay
  and timelapse export.
- **Exports**: PNG, SVG, GIF, spritesheet + JSON atlas, JPEG, WebP, ICO.

## License

MIT. See [LICENSE](LICENSE).
