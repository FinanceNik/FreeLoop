// Package frontend embeds the built React app (frontend/dist) into the binary.
// Run the frontend build before compiling anything that imports it.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built frontend with dist/ stripped.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
