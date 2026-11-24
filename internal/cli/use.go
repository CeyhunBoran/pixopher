package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// newUseCmd is `use <file>`. It edits no document, so it is a plain
// command rather than an op (D35).
func newUseCmd(st *state) *cobra.Command {
	return &cobra.Command{
		Use:   "use <file>",
		Short: "Set the current document for this directory",
		Long: "Set the current document for this directory. It is stored in " + engine.CurrentFile +
			" and used when neither -f nor $PIXOPHER_FILE is given.",
		Example: "  pxo use hero.pxo",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stored, err := engine.Use(args[0])
			if err != nil {
				return err
			}
			if st.jsonOn(cmd, nil) {
				return writeJSON(cmd.OutOrStdout(), struct {
					OK   bool   `json:"ok"`
					File string `json:"file"`
				}{true, stored})
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "using %s\n", stored); err != nil {
				return &errs.Error{Code: errs.IO, Message: err.Error()}
			}
			return nil
		},
	}
}
