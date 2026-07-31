package files

import (
	"os"
	"time"
)

// Entry is one directory listing row for local or remote panes.
type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}
