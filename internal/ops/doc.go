// Package ops is the op registry. Each op is a spec (args, docs) plus an Apply
// function that edits a document through a recording Ctx, which yields the
// step's Change (docs/decisions.md, D30).
package ops
