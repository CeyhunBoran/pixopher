package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/export"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/render"
)

// export changes no document and writes a file outside it, so it is a
// plain command, not a registry op; MCP gets it as its own tool (D58). Its
// arguments parse like a generated command's (D37).
var exportSpec = ops.Spec{
	Name:    "export",
	Summary: "Write the document as an image",
	Args: []ops.ArgSpec{
		{Name: "out", Kind: ops.KindString, Required: true, Help: "output file; the format comes from its extension (.png)"},
		{Name: "scale", Kind: ops.KindInt, Flag: true, Default: "1", Help: fmt.Sprintf("integer nearest-neighbor upscale, 1 to %d", export.MaxScale)},
		{Name: "rgba", Kind: ops.KindBool, Flag: true, Help: fmt.Sprintf("write truecolor even with %d colors or fewer", export.MaxIndexed)},
	},
	Examples: []string{"export hero.png --scale 8", "export hero.pxo out/hero@4x.png --scale 4 --json"},
}

func newExportCmd(st *state) *cobra.Command {
	return historyCmd(st, exportSpec, []ops.ArgSpec{fileFlag, jsonFlag}, st.runExport)
}

// exportResult is the --json output of export.
type exportResult struct {
	OK       bool     `json:"ok"`
	Written  []string `json:"written"`
	Warnings []string `json:"warnings"`
}

func (st *state) runExport(cmd *cobra.Command, args, _ ops.Args, path string) error {
	out, scale := args.String("out"), args.Int("scale")
	if ext := strings.ToLower(filepath.Ext(out)); ext != ".png" {
		return errs.New(errs.UnsupportedFormat, "can't export to %q: unknown format %q", out, ext).
			WithHint("use a .png file; more formats come later").WithArg("out")
	}
	if scale < 1 || scale > export.MaxScale {
		return errs.New(errs.Usage, "scale must be 1 to %d, not %d", export.MaxScale, scale).WithArg("scale")
	}
	d, err := st.cfg.backend.Load(path)
	if err != nil {
		return st.report(cmd, engine.Result{}, err)
	}
	if side := max(d.W, d.H); side*scale > export.MaxSide {
		return errs.New(errs.TooLarge, "a %dx%d canvas at --scale %d is %dx%d px", d.W, d.H, scale, d.W*scale, d.H*scale).
			WithHint("exports are at most %d px a side; the largest scale here is %d", export.MaxSide, export.MaxSide/side).
			WithArg("scale")
	}
	_, frame := ops.Target(d)
	img, warnings := render.Composite(d, frame)
	prefer := make([]doc.RGBA, len(d.Palette.Entries))
	for i, e := range d.Palette.Entries {
		prefer[i] = e.Color
	}
	var buf bytes.Buffer
	if err := export.PNG(&buf, img, export.PNGOptions{Scale: scale, RGBA: args.Bool("rgba"), Palette: prefer}); err != nil {
		return &errs.Error{Code: errs.IO, Message: err.Error()}
	}
	if dir := filepath.Dir(out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return &errs.Error{Code: errs.IO, Message: err.Error()}
		}
	}
	if err := engine.WriteFile(out, buf.Bytes()); err != nil {
		return err
	}
	if warnings == nil {
		warnings = []string{}
	}
	if *st.json {
		return writeJSON(cmd.OutOrStdout(), exportResult{true, []string{out}, warnings})
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), out); err != nil {
		return &errs.Error{Code: errs.IO, Message: err.Error()}
	}
	return nil
}
