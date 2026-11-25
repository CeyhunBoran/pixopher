// Package palettes holds the built-in palettes, embedded in the binary
// (docs/palettes.md). Each one is a .pxo file whose palette and ramps are
// copied into a new document.
package palettes

import (
	"embed"
	"path"
	"slices"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// Default is the palette of a new document when none is given (D46).
const Default = "default"

//go:embed *.pxo
var files embed.FS

// Names returns the names of the built-in palettes, sorted.
func Names() []string {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err) // embedded at build time
	}
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
	}
	slices.Sort(out)
	return out
}

// Load returns the built-in palette called name as a document that holds
// only its palette and ramps (key width 1), and false when there is none.
// Each call returns a fresh copy.
func Load(name string) (*doc.Doc, bool) {
	if !slices.Contains(Names(), name) {
		return nil, false
	}
	data, err := files.ReadFile(name + ".pxo")
	if err != nil {
		panic(err)
	}
	d, err := pxofile.Parse(data)
	if err != nil {
		panic("palettes: " + name + ": " + err.Error()) // caught by the package tests
	}
	return d, true
}
