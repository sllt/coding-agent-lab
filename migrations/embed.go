// Package migrations holds the versioned SQL executed by the control plane.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// Files is the embedded migration set.
func Files() embed.FS { return FS }
