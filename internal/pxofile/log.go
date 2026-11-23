package pxofile

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// LogPath returns the history log path of the document at path.
func LogPath(path string) string { return path + ".log" }

// Entry is one history log line: one step, with its rev and caption
// (docs/file-format.md#history-log). Field order is the JSON key order.
type Entry struct {
	Rev    int      `json:"rev"`
	Tx     string   `json:"tx"`
	TS     string   `json:"ts"` // UTC, RFC 3339, whole seconds
	Author string   `json:"author"`
	Msg    string   `json:"msg,omitempty"`
	Ops    []string `json:"ops"`
	BBox   []int    `json:"bbox,omitempty"` // [x, y, w, h] of the changed pixels
	Undo   Undo     `json:"undo"`
}

// Undo holds an entry's deltas in both directions, so it can be undone and
// redone without snapshots.
type Undo struct {
	Pixels string     `json:"pixels"` // EncodePixels; "" when no pixel changed
	Meta   []MetaSnap `json:"meta"`   // empty, or one header snapshot pair
}

// MetaSnap is a header change: the canonical .pxo text of the document
// header (no cels, rev 0) before and after the step.
type MetaSnap struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// EncodeMeta returns the log form of m, which may be nil.
func EncodeMeta(m *doc.MetaDelta) ([]MetaSnap, error) {
	if m == nil {
		return []MetaSnap{}, nil
	}
	before, err := Serialize(m.Before)
	if err != nil {
		return nil, err
	}
	after, err := Serialize(m.After)
	if err != nil {
		return nil, err
	}
	return []MetaSnap{{Before: string(before), After: string(after)}}, nil
}

// DecodeMeta parses the log form of a meta delta. It returns nil when snaps
// is empty.
func DecodeMeta(snaps []MetaSnap) (*doc.MetaDelta, error) {
	switch len(snaps) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, invalid("history entry has %d meta snapshots, want at most 1", len(snaps))
	}
	before, err := Parse([]byte(snaps[0].Before))
	if err != nil {
		return nil, err
	}
	after, err := Parse([]byte(snaps[0].After))
	if err != nil {
		return nil, err
	}
	return &doc.MetaDelta{Before: before.Header(), After: after.Header()}, nil
}

// pixelsVersion is the first byte of packed pixel deltas.
const pixelsVersion = 1

// EncodePixels packs deltas, which must be sorted by layer, frame, y and x,
// into base64 (docs/decisions.md, D31). Each cel is a group: layer, frame,
// canvas width and run count, then runs of consecutive pixels (in row-major
// order) that share the same before and after values, each as the gap from
// the previous run, the length, before and after. All numbers are unsigned
// varints. No deltas give "".
func EncodePixels(deltas []doc.PixelDelta, width int) string {
	if len(deltas) == 0 {
		return ""
	}
	type run struct{ pos, n, before, after int }
	var out []byte
	out = append(out, pixelsVersion)
	for i := 0; i < len(deltas); {
		layer, frame := deltas[i].Layer, deltas[i].Frame
		var runs []run
		for ; i < len(deltas) && deltas[i].Layer == layer && deltas[i].Frame == frame; i++ {
			p := deltas[i]
			pos := p.Y*width + p.X
			if n := len(runs); n > 0 {
				r := &runs[n-1]
				if r.pos+r.n == pos && r.before == int(p.Before) && r.after == int(p.After) {
					r.n++
					continue
				}
			}
			runs = append(runs, run{pos, 1, int(p.Before), int(p.After)})
		}
		out = binary.AppendUvarint(out, uint64(layer))
		out = binary.AppendUvarint(out, uint64(frame))
		out = binary.AppendUvarint(out, uint64(width))
		out = binary.AppendUvarint(out, uint64(len(runs)))
		end := 0
		for _, r := range runs {
			out = binary.AppendUvarint(out, uint64(r.pos-end))
			out = binary.AppendUvarint(out, uint64(r.n))
			out = binary.AppendUvarint(out, uint64(r.before))
			out = binary.AppendUvarint(out, uint64(r.after))
			end = r.pos + r.n
		}
	}
	return base64.StdEncoding.EncodeToString(out)
}

// DecodePixels unpacks EncodePixels output.
func DecodePixels(s string) ([]doc.PixelDelta, error) {
	if s == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, invalid("history entry has invalid pixel data: %v", err)
	}
	bad := func(what string) error { return invalid("history entry has invalid pixel data: %s", what) }
	if len(data) == 0 || data[0] != pixelsVersion {
		return nil, bad("unknown encoding version")
	}
	r := bytes.NewReader(data[1:])
	const maxPos = doc.MaxSize * doc.MaxSize
	next := func(limit uint64) (int, error) {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return 0, bad("truncated")
		}
		if v > limit {
			return 0, bad("value out of range")
		}
		return int(v), nil
	}
	var out []doc.PixelDelta
	for r.Len() > 0 {
		var g [4]int
		for i, lim := range []uint64{1 << 16, 1 << 16, doc.MaxSize, maxPos} {
			if g[i], err = next(lim); err != nil {
				return nil, err
			}
		}
		layer, frame, width, runs := g[0], g[1], g[2], g[3]
		if width == 0 {
			return nil, bad("zero width")
		}
		pos := 0
		for range runs {
			var f [4]int
			for i, lim := range []uint64{maxPos, maxPos, 1<<16 - 1, 1<<16 - 1} {
				if f[i], err = next(lim); err != nil {
					return nil, err
				}
			}
			pos += f[0]
			if f[1] == 0 || pos+f[1] > maxPos {
				return nil, bad("run out of range")
			}
			for range f[1] {
				out = append(out, doc.PixelDelta{
					Layer: layer, Frame: frame, X: pos % width, Y: pos / width,
					Before: doc.Index(f[2]), After: doc.Index(f[3]),
				})
				pos++
			}
		}
	}
	return out, nil
}

// MarshalEntries returns entries as JSON Lines. HTML characters in messages
// are not escaped.
func MarshalEntries(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for i := range entries {
		e := entries[i]
		if e.Ops == nil {
			e.Ops = []string{}
		}
		if e.Undo.Meta == nil {
			e.Undo.Meta = []MetaSnap{}
		}
		if err := enc.Encode(&e); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// AppendLog appends entries to the log at path in a single write, creating
// the file if needed. A partial last line left by a crash during an earlier
// append is cut off first, so the log stays one JSON object per line.
func AppendLog(path string, entries []Entry) error {
	data, err := MarshalEntries(entries)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return ioErr(err)
	}
	if err := repairTail(f); err != nil {
		_ = f.Close()
		return ioErr(err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return ioErr(err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return ioErr(err)
	}
	return ioErr(f.Close())
}

// repairTail makes f end in a newline. A last line that is valid JSON (say,
// from a hand edit) only gets its newline; anything else after the last
// newline is a fragment from an interrupted append and is cut off.
func repairTail(f *os.File) error {
	cut, err := lastLineStart(f)
	if err != nil || cut < 0 {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	tail := make([]byte, st.Size()-cut)
	if _, err := f.ReadAt(tail, cut); err != nil {
		return err
	}
	if json.Valid(tail) {
		_, err := f.WriteAt([]byte{'\n'}, st.Size())
		return err
	}
	return f.Truncate(cut)
}

// lastLineStart returns the offset just after the last newline of f, or -1
// when f is empty or already ends in a newline.
func lastLineStart(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := st.Size()
	if size == 0 {
		return -1, nil
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], size-1); err != nil {
		return 0, err
	}
	if last[0] == '\n' {
		return -1, nil
	}
	const chunk = 4096
	buf := make([]byte, chunk)
	for end := size; end > 0; {
		start := max(end-chunk, 0)
		n, err := f.ReadAt(buf[:end-start], start)
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if i := bytes.LastIndexByte(buf[:n], '\n'); i >= 0 {
			return start + int64(i) + 1, nil
		}
		end = start
	}
	return 0, nil
}

// ReadLog reads every entry of the log at path. A missing log is not an
// error: the document then has no history. An incomplete last line (a
// crash during append) is ignored; any other bad line is a parse_error.
func ReadLog(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ioErr(err)
	}
	var out []Entry
	for n := 1; len(data) > 0; n++ {
		line, rest, complete := bytes.Cut(data, []byte{'\n'})
		data = rest
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			if !complete {
				break
			}
			return nil, invalid("%s line %d: %v", path, n, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func ioErr(err error) error {
	if err == nil {
		return nil
	}
	return &errs.Error{Code: errs.IO, Message: err.Error()}
}
