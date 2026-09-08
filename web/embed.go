// Package webassets contains the frontend included in release binaries.
package webassets

import (
	"embed"
	"fmt"
	"io/fs"
)

// Files excludes Go sources and local user data.
//
//go:embed index.html app.js style.css assets
var Files embed.FS

func Validate(files fs.FS) error {
	for _, name := range []string{"index.html", "app.js", "style.css", "assets/vue.global.prod.js"} {
		info, err := fs.Stat(files, name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", name)
		}
	}
	return nil
}
