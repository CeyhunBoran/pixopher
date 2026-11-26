# Architecture

## Principles

1. **The `.pxo` file is the source of truth.** Anything in memory is a cache.
2. **One operation registry.** Every drawing/editing command is defined once
   as an *op*: name, typed arguments, docs, and an apply function. The CLI,
   batch scripts, MCP tools, the `guide` command and the JSON schema are all
   generated from that registry, so they can never drift apart.
3. **Every change is a transaction.** A command or a batch (`run`) applies
   atomically. Each step inside it bumps the revision and writes one history
   entry with its caption. All entries of one transaction share a `tx` id,
   and `undo` reverts a whole transaction (`--step` reverts a single entry).
   See [file-format.md](file-format.md#history-log-wip).
4. **Pure Go, no CGO.** It cross-compiles to linux/macOS/windows on amd64 and
   arm64.
5. **Deterministic.** The same document plus the same ops gives the same
   pixels on every platform. Anything random (dithering noise, jitter) takes
   a seed.

## Hybrid state model

```
                 ┌──────────────────────────┐
  pxo <cmd> ───► │  engine (in-process)     │──► sprite.pxo        (atomic write)
                 │  lock → load → apply →   │──► sprite.pxo.log    (append history)
                 │  save → log              │
                 └──────────────────────────┘
                          ▲  if a server is running for this file,
                          │  the CLI forwards the op to it instead
                 ┌──────────────────────────┐
                 │  pixopher serve          │  keeps docs hot in memory,
                 │  (optional daemon)       │  writes through to disk,
                 │                          │  pushes events to viewers
                 └──────────────────────────┘
                     │ SSE / socket events
          ┌──────────┴──────────┐
     pixopher watch        web viewer
     (terminal)            (browser)
```

### Direct mode (default, no daemon)

Each command:

1. Resolves the target file (see [cli.md](cli.md#file-resolution-done)).
2. Takes an exclusive advisory lock on `sprite.pxo.lock` (`flock` on Unix,
   `LockFileEx` on Windows) with a short timeout. The lock file stays on
   disk (D32).
3. Loads and parses the `.pxo`.
4. Checks `--expect-rev` if given (optimistic concurrency).
5. Applies the ops in memory and computes the change set (changed pixels,
   bounding box, inverse ops).
6. Writes the new file to a temp file in the same directory, then renames it
   over the original (atomic on POSIX, best-effort on Windows).
7. Appends a history entry to `sprite.pxo.log`.
8. Releases the lock and prints the result.

Viewers in direct mode watch the file with `fsnotify` and re-render on change.
They read the newest log entry to know what changed and to show the caption.

The target cost per command is under 15 ms for a 64x64 document, so direct
mode is fine for most agent work. That matters because agents batch.

### Server mode (`pixopher serve`)

`pixopher serve [files...]` starts a daemon that:

- holds open documents in memory (no parse per command),
- listens on a local socket: a Unix socket at
  `$XDG_RUNTIME_DIR/pixopher/<hash-of-abs-path>.sock` (fallback
  `os.TempDir()`), or `127.0.0.1` TCP with a token on Windows,
- writes through to disk after every transaction, so the file stays the
  source of truth and a crash loses nothing,
- serves the web viewer and the SSE event stream over HTTP on `127.0.0.1`
  only.

The CLI auto-detects a server for the resolved file and forwards ops over
the socket. If none is running, it falls back to direct mode. Output is
identical in both modes. `--no-server` forces direct mode.

The server also takes the file lock while it holds a document, and reloads if
the file changes on disk from outside (for example, a human hand-editing it).

### Concurrency

- **Locking** serializes writers on one machine.
- **Revisions**: every document has a monotonically increasing `rev`.
  `--expect-rev N` makes a write fail with `rev_conflict` if someone else
  wrote in between. Agents that read, think and then write should use it.
- **Authors**: each history entry records an `author` (from `--as`, or
  `PIXOPHER_AUTHOR`, default `cli`). Viewers can color captions per author.
  This allows several agents to collaborate on one sprite, for example one
  drawing and one doing shading.

## Package layout

```
cmd/pixopher/          main; also builds the `pxo` alias (same binary, argv[0] agnostic)
internal/
  errs/       stable error codes (*errs.Error), exit codes; no internal imports
  doc/        document model: Doc, Palette, Ramp, Layer, Frame, Cel, Region, Anchor, Part
  pxofile/    .pxo parse + serialize (TOML + grid/RLE cels), history log I/O
  color/      color parsing, OKLab/OKLCH math, ramp generation, quantization, ΔE
  raster/     pure algorithms: Bresenham, ellipse, polygon fill, flood fill, transforms
  ops/        the op registry; each op = spec (args, docs) + Apply(ctx, args), recorded into a Change (D30)
  fx/         outline, shade, recolor, dither, gradient (used by ops)
  script/     batch script lexer/parser → []ops.Call
  engine/     transaction: lock, load, apply, save, log; Backend interface (direct | server client)
  history/    undo/redo, checkpoints, replay iteration over the log
  render/     compositing (layers, opacity, blend), scaling, preview sheets, ANSI/kitty/sixel encoders
  lint/       lint rules + runner
  export/     png, svg, gif, sheet+json, jpeg, webp, ico
  importer/   png/jpeg/gif → doc, palette files (.hex .gpl .pal .ase), quantize/pixelate
  server/     serve daemon: socket RPC, HTTP, SSE, document cache
  webui/      embedded static web viewer (embed.FS), no build step required
  tui/        `watch` terminal viewer
  mcp/        MCP stdio server mapping registry ops to tools
  guide/      agent manual, generated from the registry + hand-written craft notes
  cli/        cobra command tree generated from the registry + non-op commands
  golden/     test helper: the -update flag and golden file comparison
palettes/     built-in palette files (embedded)
testdata/     golden .pxo files and expected PNG/SVG outputs
```

Dependency direction (no cycles): `doc` ← `color`, `raster` ← `fx` ← `ops` ←
`script`, `history` ← `engine` ← (`cli`, `server`, `mcp`, `tui`). `history`
uses `ops` and `pxofile` (for the log entries). `render`, `export`,
`lint` and `importer` depend on `doc` and `color` only. `pxofile` depends on
`doc` and `color`. `palettes` (the embedded built-ins) depends on `doc` and
`pxofile`, and `ops` imports it (D53). Every package may import `errs`.

## The op registry

```go
type Spec struct {
    Name     string          // "rect"
    Group    string          // "draw"
    Summary  string
    Args     []ArgSpec       // positional + flags, typed: Point, Size, Rect, Area, Color, Int, Enum, String, Grid
    Examples []string
    Mutates  bool            // false for show/get/lint style ops
    Creates  bool            // new: makes the document instead of changing one (D39, D46)
}

type Op interface{ Spec() Spec }

type Mutator interface { // Mutates
    Op
    Apply(ctx *Ctx, args Args) error // Ctx has doc, target layer/frame (later: symmetry, selection)
}

type Observer interface { // !Mutates: info, show, get, stats, lint (D38, D52)
    Op
    Observe(d *doc.Doc, args Args) (Report, error) // no Ctx, no lock, no log
}

type Report interface{ Text() string } // human form; the JSON form is the value itself

// Ctx records every pixel written through ctx.Set and snapshots the header;
// ctx.Finish() returns the step's Change (D30).
type Change struct {
    Pixels []doc.PixelDelta // layer, frame, x, y, before, after; sorted
    Meta   *doc.MetaDelta   // header snapshots before/after, nil if unchanged
    BBox   doc.Rect
}
```

`ops.Revert` and `ops.Reapply` apply a `Change` backwards or forwards, which
is all that undo and redo need. The log encoding of a `Change` is in D31.

- The CLI turns each `Spec` into a cobra command (D37).
- `ops.Parse` turns tokens into `Args`, and `ops.Format` turns `Args` back
  into the canonical line that the log stores (D34). The CLI uses them, and
  the batch parser will turn each script line into the same `Args` with them.
- The MCP server exposes a small set of tools that call into the registry (see
  [agent-workflow.md](agent-workflow.md#mcp-server)).
- `pixopher schema --json` dumps all specs for agents and tooling.
- `pixopher guide` renders specs and examples into an agent manual.

## Libraries (initial picks, revisit in decisions.md if changed)

| Need | Library |
| --- | --- |
| CLI | `github.com/spf13/cobra` |
| TOML | `github.com/BurntSushi/toml` (parse only; the writer is hand-written, D23) |
| TUI | `github.com/charmbracelet/bubbletea`, `lipgloss` |
| File watching | `github.com/fsnotify/fsnotify` |
| File locks | `golang.org/x/sys` (`flock`, `LockFileEx`, D33) |
| MCP | `github.com/modelcontextprotocol/go-sdk` |
| PNG/GIF/JPEG | Go stdlib `image/*` |
| WebP (lossless encode) | pure Go encoder, see [open-questions.md](open-questions.md) |
| Sixel | `github.com/mattn/go-sixel` (or hand-rolled) |
| Kitty graphics | hand-rolled (the protocol is simple) |

## Testing strategy

- **Golden tests**: `testdata/*.pxo` + script → expected `.pxo` and PNG,
  compared byte-for-byte. Update with `go test ./... -update`.
- **Round-trip**: parse → serialize → parse is identity for every golden file.
- **Op property tests**: apply then undo is identity for every mutating op,
  both in memory and through the engine's history log. Guard tests fail
  when a registered op has no golden case or no undo identity case.
- **Budget**: each command of the M1 flow runs under 15 ms on a 64x64
  document (`PIXOPHER_BUDGET=1`, Linux CI, D61).
- **Lint fixtures**: one small `.pxo` per rule with expected findings.
- **CLI tests**: run the command tree in-process and assert stdout/JSON/exit
  codes.
- CI: `go vet`, `staticcheck`/`golangci-lint`, `go test -race ./...` on
  linux/macOS/windows.
