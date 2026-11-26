package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/history"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// DefaultAuthor is the author of history entries when none is given.
const DefaultAuthor = "cli"

// DefaultLockTimeout is how long a writer waits for the file lock.
const DefaultLockTimeout = 2 * time.Second

// Options are the per-transaction settings behind the common flags of
// mutating commands (docs/cli.md#common-flags-all-mutating-commands).
type Options struct {
	Author      string           // --as; DefaultAuthor when empty
	ExpectRev   *int             // --expect-rev; nil when not given
	DryRun      bool             // --dry-run: compute and report, write nothing
	Force       bool             // --force for creating ops: replace an existing document
	Message     string           // -m for Undo; Apply's steps carry their own captions
	LockTimeout time.Duration    // DefaultLockTimeout when zero
	Now         func() time.Time // clock for entry timestamps; time.Now when nil

	// ContinueOnError makes Apply skip a call that fails, undoing what it
	// wrote, and report it as a warning instead of failing (run
	// --continue-on-error).
	ContinueOnError bool
	// OnOp, when set, gets one event per call as it applies, and Apply
	// waits Pace after each (run --pace). Direct mode has no live viewer
	// to send them to, so the CLI leaves it nil there and never waits
	// (D63).
	OnOp func(OpEvent)
	Pace time.Duration
}

// OpEvent is one call of a transaction as it applies, for live viewers:
// the `op` event of docs/live-view.md#event-model.
type OpEvent struct {
	Rev    int              // the rev of the step the call is in, once that step is written
	Op     string           // the call's canonical line
	Pixels []doc.PixelDelta // what the call wrote: Before is the value at the start of its step
}

// Step is one history entry's worth of calls, with its caption.
type Step struct {
	Msg   string
	Calls []ops.Call
}

// Result is the outcome of a transaction. Its JSON form is the --json
// output of a mutating command (docs/cli.md#output-done).
type Result struct {
	OK       bool     `json:"ok"`
	Rev      int      `json:"rev"`               // rev after the transaction (would-be rev for a dry run)
	Changed  int      `json:"changed"`           // pixels that differ from before the transaction
	BBox     []int    `json:"bbox,omitempty"`    // [x, y, w, h] of the changed pixels
	Undone   []Target `json:"undone,omitempty"`  // undo: the entries reverted, in order
	Redone   []Target `json:"redone,omitempty"`  // redo: the entries applied again, in order
	Warnings []string `json:"warnings"`          // never null
	DryRun   bool     `json:"dry_run,omitempty"` // true when nothing was written
}

// Target names a history entry that undo reverted or redo applied again.
type Target struct {
	Rev int    `json:"rev"`
	Tx  string `json:"tx"`
}

// Backend runs transactions on documents: in process (Direct) or, from M7,
// through a running server.
type Backend interface {
	// Apply runs steps on the document at path as one transaction.
	Apply(path string, steps []Step, opt Options) (Result, error)
	// Create makes the document at path with a creating op (D39).
	Create(path string, call ops.Call, opt Options) (Result, error)
	// Undo reverts (or, with req.Redo, re-applies) history entries of the
	// document at path as one transaction.
	Undo(path string, req history.Request, opt Options) (Result, error)
	// Load reads the document at path.
	Load(path string) (*doc.Doc, error)
	// History reads the document at path and its effective history (D49).
	History(path string) (*doc.Doc, history.Chain, error)
}

// Direct is the in-process backend (docs/architecture.md#direct-mode-default-no-daemon).
type Direct struct{}

var _ Backend = Direct{}

// Load reads and parses the document at path. It takes no lock: writers
// replace the file atomically, so a reader never sees a partial write.
func (Direct) Load(path string) (*doc.Doc, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, noFile(path)
	}
	if err != nil {
		return nil, ioErr(err)
	}
	return pxofile.Parse(data)
}

func noFile(path string) error {
	return errs.New(errs.NoFile, "%s does not exist", path).
		WithHint("create it with `pxo new %s --size 32x32`", path)
}

// Apply runs steps as one transaction: lock, load, check --expect-rev, apply
// every step, write the document atomically, append one history entry per
// step, unlock. Every step that changes something bumps the rev; a step that
// changes nothing writes no entry, and a transaction that changes nothing
// writes no file. If any call fails, nothing is written. A dry run takes no
// lock and writes nothing, but reports the same result and errors.
func (b Direct) Apply(path string, steps []Step, opt Options) (Result, error) {
	d, release, err := b.begin(path, &opt)
	if err != nil {
		return Result{}, err
	}
	defer release()

	ts := opt.Now().UTC().Format(time.RFC3339)
	var (
		changes  []ops.Change
		entries  []pxofile.Entry
		tx       string
		warnings = []string{}
	)
	// The target cel carries over from one step to the next, so an @set
	// holds until the next one (D62).
	layer, frame := ops.Target(d)
	for _, st := range steps {
		ctx := ops.NewCtx(d, layer, frame)
		var lines []string
		if dl, df := ops.Target(d); layer != dl || frame != df {
			// Each entry names its own target, so it replays on its own.
			lines = append(lines, ops.SetLine(d, layer, frame))
		}
		for _, c := range st.Calls {
			skipped, err := applyCall(ctx, c, &opt)
			if err != nil {
				return Result{}, err
			}
			if skipped != "" {
				warnings = append(warnings, skipped)
				continue
			}
			lines = append(lines, c.Line)
			if opt.OnOp != nil && !ops.IsSet(c) {
				opt.OnOp(OpEvent{Rev: d.Rev + 1, Op: c.Line, Pixels: ctx.Written()})
				time.Sleep(opt.Pace)
			}
		}
		layer, frame = ctx.Layer, ctx.Frame
		ch := ctx.Finish()
		if ch.Empty() {
			continue
		}
		d.Rev++
		if tx == "" {
			tx = TxID(d.Rev)
		}
		e, err := entry(d, ch, lines, st.Msg, tx, ts, opt.Author)
		if err != nil {
			return Result{}, err
		}
		changes = append(changes, ch)
		entries = append(entries, e)
	}

	net := ops.NetPixels(changes)
	res := Result{OK: true, Rev: d.Rev, Changed: len(net), Warnings: warnings, DryRun: opt.DryRun}
	if len(net) > 0 {
		res.BBox = rectSlice(ops.BBox(net))
	}
	if len(entries) == 0 {
		return res, nil
	}
	return res, commit(path, d, entries, opt.DryRun)
}

// applyCall runs one call on ctx. A failing call from a script line fails as a
// script_error naming the line. With opt.ContinueOnError, its writes are
// undone instead and apply returns the warning that reports the skip.
func applyCall(ctx *ops.Ctx, c ops.Call, opt *Options) (skipped string, err error) {
	if opt.ContinueOnError || opt.OnOp != nil {
		ctx.Mark()
	}
	err = c.Op.Apply(ctx, c.Args)
	if err == nil {
		return "", nil
	}
	if !opt.ContinueOnError {
		if c.LineNo > 0 {
			err = errs.AtLine(c.LineNo, err)
		}
		return "", err
	}
	ctx.Rollback()
	return skipWarning(c.LineNo, err), nil
}

// skipWarning is the warning for a call skipped by --continue-on-error:
// `line 7 skipped: unknown_color: unknown color "inc" (did you mean "ink"?)`.
func skipWarning(line int, err error) string {
	w := "skipped: "
	if line > 0 {
		w = fmt.Sprintf("line %d skipped: ", line)
	}
	e, ok := errors.AsType[*errs.Error](err)
	if !ok {
		return w + err.Error()
	}
	w += fmt.Sprintf("%s: %s", e.Code, e.Message)
	if e.Hint != "" {
		w += " (" + e.Hint + ")"
	}
	return w
}

// Undo runs an undo or redo as one transaction under the lock: it reads the
// log, builds the document's chain (D49), and writes one entry per reverted
// or re-applied entry, each bumping the rev. Entries are written even when a
// forced step changes nothing, because they change what can be undone next.
// opt.Author is the author of the new entries and, unless req.Author is
// set, whose history is undone.
func (b Direct) Undo(path string, req history.Request, opt Options) (Result, error) {
	d, release, err := b.begin(path, &opt)
	if err != nil {
		return Result{}, err
	}
	defer release()
	log, err := pxofile.ReadLog(pxofile.LogPath(path))
	if err != nil {
		return Result{}, err
	}
	chain := history.Build(log, d.Rev)
	if req.Author == "" {
		req.Author = opt.Author
	}
	steps, err := history.Run(d, chain, req)
	if err != nil {
		return Result{}, err
	}
	kind := pxofile.KindUndo
	if req.Redo {
		kind = pxofile.KindRedo
	}
	ts := opt.Now().UTC().Format(time.RFC3339)
	tx := TxID(d.Rev + 1)
	var (
		changes []ops.Change
		entries []pxofile.Entry
		targets []Target
	)
	for _, st := range steps {
		d.Rev++
		e, err := entry(d, st.Change, nil, opt.Message, tx, ts, opt.Author)
		if err != nil {
			return Result{}, err
		}
		e.Kind, e.Target = kind, st.Target.Rev
		changes = append(changes, st.Change)
		entries = append(entries, e)
		targets = append(targets, Target{Rev: st.Target.Rev, Tx: st.Target.Tx})
	}
	net := ops.NetPixels(changes)
	res := Result{OK: true, Rev: d.Rev, Changed: len(net), Warnings: chain.Warnings, DryRun: opt.DryRun}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	if len(net) > 0 {
		res.BBox = rectSlice(ops.BBox(net))
	}
	if req.Redo {
		res.Redone = targets
	} else {
		res.Undone = targets
	}
	return res, commit(path, d, entries, opt.DryRun)
}

// History reads the document at path and its effective history. Like Load,
// it takes no lock.
func (b Direct) History(path string) (*doc.Doc, history.Chain, error) {
	d, err := b.Load(path)
	if err != nil {
		return nil, history.Chain{}, err
	}
	log, err := pxofile.ReadLog(pxofile.LogPath(path))
	if err != nil {
		return nil, history.Chain{}, err
	}
	return d, history.Build(log, d.Rev), nil
}

// begin fills in opt's defaults, takes the lock (not for a dry run), loads
// the document and checks --expect-rev. The caller must call release.
func (b Direct) begin(path string, opt *Options) (d *doc.Doc, release func(), err error) {
	if opt.Author == "" {
		opt.Author = DefaultAuthor
	}
	if opt.LockTimeout == 0 {
		opt.LockTimeout = DefaultLockTimeout
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	release = func() {}
	if !opt.DryRun {
		// Don't leave a stray lock file next to a document that isn't there.
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil, nil, noFile(path)
		}
		l, err := acquire(LockPath(path), opt.LockTimeout)
		if err != nil {
			return nil, nil, err
		}
		release = func() { _ = l.release() }
	}
	if d, err = b.Load(path); err != nil {
		release()
		return nil, nil, err
	}
	if opt.ExpectRev != nil && *opt.ExpectRev != d.Rev {
		release()
		return nil, nil, errs.New(errs.RevConflict, "%s is at rev %d, not %d", path, d.Rev, *opt.ExpectRev).
			WithHint("another write happened; look at the document again, then retry with --expect-rev %d", d.Rev).
			WithArg("expect-rev")
	}
	return d, release, nil
}

// commit writes d and appends entries to its log, unless dryRun is set.
func commit(path string, d *doc.Doc, entries []pxofile.Entry, dryRun bool) error {
	data, err := pxofile.Serialize(d)
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	// The document is the source of truth, so it is written first. If the
	// process dies before the log append, the document is intact and only
	// those steps lack undo history.
	if err := writeAtomic(path, data); err != nil {
		return err
	}
	return pxofile.AppendLog(pxofile.LogPath(path), entries)
}

// Create runs a creating op (D39) and writes its document at rev 0, with no
// history entry. It fails with file_exists when path exists, unless
// opt.Force is set; then the document is replaced and its history log is
// removed, because the old entries don't apply to the new document (D46).
// Changed counts the pixels that differ from an empty canvas (--bg).
func (b Direct) Create(path string, call ops.Call, opt Options) (Result, error) {
	if opt.LockTimeout == 0 {
		opt.LockTimeout = DefaultLockTimeout
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	c, ok := call.Op.(ops.Creator)
	if !ok {
		return Result{}, fmt.Errorf("engine: %s is not a creating op", call.Op.Spec().Name)
	}
	d, err := c.Create(call.Args)
	if err != nil {
		return Result{}, err
	}
	d.Meta.Created = opt.Now().UTC().Truncate(time.Second)
	layer, frame := ops.Target(d)
	ctx := ops.NewCtx(d, layer, frame)
	if err := c.Apply(ctx, call.Args); err != nil {
		return Result{}, err
	}
	ch := ctx.Finish()
	res := Result{OK: true, Rev: d.Rev, Changed: len(ch.Pixels), Warnings: []string{}, DryRun: opt.DryRun}
	if len(ch.Pixels) > 0 {
		res.BBox = rectSlice(ch.BBox)
	}
	data, err := pxofile.Serialize(d)
	if err != nil {
		return Result{}, err
	}
	// Checked before the lock, so a failure leaves no lock file behind and a
	// dry run fails the same way a real run would.
	if err := checkCreate(path, opt.Force); err != nil {
		return Result{}, err
	}
	if opt.DryRun {
		return res, nil
	}
	// The lock keeps two writers from creating or replacing the same
	// document at once. The existence check runs again under it, because
	// another writer may have created the file in between.
	l, err := acquire(LockPath(path), opt.LockTimeout)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = l.release() }()
	if err := checkCreate(path, opt.Force); err != nil {
		return Result{}, err
	}
	if err := writeAtomic(path, data); err != nil {
		return Result{}, err
	}
	if err := os.Remove(pxofile.LogPath(path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Result{}, ioErr(err)
	}
	return res, nil
}

// checkCreate fails with io when the directory for path is missing, and
// with file_exists when path exists, unless force is set and path is not a
// directory.
func checkCreate(path string, force bool) error {
	dir := filepath.Dir(path)
	if st, err := os.Stat(dir); err != nil {
		return ioErr(err)
	} else if !st.IsDir() {
		return errs.New(errs.IO, "%s is not a directory", dir)
	}
	st, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return ioErr(err)
	case st.IsDir():
		return errs.New(errs.FileExists, "%s is a directory", path).WithArg("file")
	case !force:
		return errs.New(errs.FileExists, "%s already exists", path).
			WithHint("pass --force to replace it (its history is removed too)").WithArg("file")
	}
	return nil
}

// TxID returns the transaction id for a transaction whose first entry has
// rev: "t_" and the rev, zero-padded to four digits (D32).
func TxID(rev int) string { return fmt.Sprintf("t_%04d", rev) }

func entry(d *doc.Doc, ch ops.Change, lines []string, msg, tx, ts, author string) (pxofile.Entry, error) {
	meta, err := pxofile.EncodeMeta(ch.Meta)
	if err != nil {
		return pxofile.Entry{}, err
	}
	e := pxofile.Entry{
		Rev: d.Rev, Tx: tx, TS: ts, Author: author, Msg: msg, Ops: lines,
		Undo: pxofile.Undo{Pixels: pxofile.EncodePixels(ch.Pixels, d.W), Meta: meta},
	}
	if len(ch.Pixels) > 0 {
		e.BBox = rectSlice(ch.BBox)
	}
	return e, nil
}

func rectSlice(r doc.Rect) []int { return []int{r.X, r.Y, r.W, r.H} }

func ioErr(err error) error {
	if err == nil {
		return nil
	}
	return &errs.Error{Code: errs.IO, Message: err.Error()}
}
