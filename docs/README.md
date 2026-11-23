# pixopher docs

These documents are the **source of truth** for what pixopher is and how it is
built. Code follows the docs. When a design changes, change the doc in the same
commit as the code.

pixopher is a pixel art CLI written in Go. It is built so that AI coding agents
can draw with it efficiently, and so that a human can watch the drawing happen
live in a terminal or browser.

## Reading order

| Doc | What it answers |
| --- | --- |
| [vision.md](vision.md) | Why pixopher exists, goals, non-goals, prior art |
| [architecture.md](architecture.md) | Hybrid file/daemon state model, packages, the op registry, concurrency |
| [file-format.md](file-format.md) | The `.pxo` document format and its history sidecar |
| [palettes.md](palettes.md) | Palette-first color model, ramps, color syntax, palette library |
| [cli.md](cli.md) | Full command surface, addressing, output, error codes |
| [drawing.md](drawing.md) | Primitives, symmetry, parts, regions and anchors, outline, shading, recolor |
| [agent-workflow.md](agent-workflow.md) | Batch scripts, grid paint/read, `look`, `guide`, MCP server |
| [live-view.md](live-view.md) | `watch` terminal viewer, `serve` web viewer, captions, replay and timelapse |
| [lint.md](lint.md) | Pixel art lint rules |
| [export-import.md](export-import.md) | PNG, SVG, GIF, spritesheet, JPEG, WebP, ICO, and imports |
| [roadmap.md](roadmap.md) | Milestones with acceptance criteria, in build order |
| [decisions.md](decisions.md) | Decision log (why things are the way they are) |
| [open-questions.md](open-questions.md) | Things not decided yet |

## How to work from these docs (for agents and humans)

1. Read `vision.md` and `architecture.md` once.
2. Pick the first unfinished milestone in `roadmap.md`. Its acceptance criteria
   are the definition of done.
3. Read the docs that milestone links to. Do not implement features from later
   milestones "while you are there".
4. If you hit a question the docs don't answer, add it to `open-questions.md`
   instead of silently deciding. If you must decide to make progress, record
   the decision in `decisions.md`.
5. When a milestone is done, tick its checkboxes in `roadmap.md` in the same
   commit.

## Status markers

Feature sections use these markers so it is clear what exists:

- **[planned]**: designed, not built
- **[wip]**: partially built
- **[done]**: built and tested
- **[idea]**: not committed to, might be dropped

Everything is **[planned]** unless marked otherwise.
