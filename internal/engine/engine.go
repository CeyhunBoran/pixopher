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
	LockTimeout time.Duration    // DefaultLockTimeout when zero
	Now         func() time.Time // clock for entry timestamps; time.Now when nil
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
	Warnings []string `json:"warnings"`          // never null
	DryRun   bool     `json:"dry_run,omitempty"` // true when nothing was written
}

// Backend runs transactions on documents: in process (Direct) or, from M7,
// through a running server.
type Backend interface {
	// Apply runs steps on the document at path as one transaction.
	Apply(path string, steps []Step, opt Options) (Result, error)
	// Create makes the document at path with a creating op (D39).
	Create(path string, call ops.Call, opt Options) (Result, error)
	// Load reads the document at path.
	Load(path string) (*doc.Doc, error)
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
	if opt.Author == "" {
		opt.Author = DefaultAuthor
	}
	if opt.LockTimeout == 0 {
		opt.LockTimeout = DefaultLockTimeout
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if !opt.DryRun {
		// Don't leave a stray lock file next to a document that isn't there.
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return Result{}, noFile(path)
		}
		l, err := acquire(LockPath(path), opt.LockTimeout)
		if err != nil {
			return Result{}, err
		}
		defer func() { _ = l.release() }()
	}
	d, err := b.Load(path)
	if err != nil {
		return Result{}, err
	}
	if opt.ExpectRev != nil && *opt.ExpectRev != d.Rev {
		return Result{}, errs.New(errs.RevConflict, "%s is at rev %d, not %d", path, d.Rev, *opt.ExpectRev).
			WithHint("another write happened; look at the document again, then retry with --expect-rev %d", d.Rev).
			WithArg("expect-rev")
	}

	ts := opt.Now().UTC().Format(time.RFC3339)
	var (
		changes []ops.Change
		entries []pxofile.Entry
		tx      string
	)
	for _, st := range steps {
		// The target is the first layer and frame until --layer and
		// --frame land (M4).
		ctx := ops.NewCtx(d, 0, 0)
		lines := make([]string, len(st.Calls))
		for i, c := range st.Calls {
			if err := c.Op.Apply(ctx, c.Args); err != nil {
				return Result{}, err
			}
			lines[i] = c.Line
		}
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
	res := Result{OK: true, Rev: d.Rev, Changed: len(net), Warnings: []string{}, DryRun: opt.DryRun}
	if len(net) > 0 {
		res.BBox = rectSlice(ops.BBox(net))
	}
	if len(entries) == 0 {
		return res, nil
	}
	data, err := pxofile.Serialize(d)
	if err != nil {
		return Result{}, err
	}
	if opt.DryRun {
		return res, nil
	}
	// The document is the source of truth, so it is written first. If the
	// process dies before the log append, the document is intact and only
	// those steps lack undo history.
	if err := writeAtomic(path, data); err != nil {
		return Result{}, err
	}
	if err := pxofile.AppendLog(pxofile.LogPath(path), entries); err != nil {
		return Result{}, err
	}
	return res, nil
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
	ctx := ops.NewCtx(d, 0, 0)
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
