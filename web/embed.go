// Package web holds the embedded single-page frontend that the serve command
// answers on GET /. Embedding (rather than reading from disk) keeps the binary
// self-contained for Render-style deploys.
package web

import _ "embed"

//go:embed index.html
var Index []byte
