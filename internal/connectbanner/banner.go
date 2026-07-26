package connectbanner

import (
	"io"
	"strings"
)

// Banner is shown before an interactive SSH session starts.
const Banner = "\x1b[1;38;2;139;92;246m BAST \x1b[0m  Connecting to server…\r\n" +
	"\x1b[38;2;107;114;128m Stuck? Press Enter, then ~. to return to Bast.\x1b[0m\r\n\r\n"

const (
	statusPrefix = "\x1b[38;2;107;114;128m "
	statusSuffix = "\x1b[0m\r\n"
)

// Write writes the connection banner to w.
func Write(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = io.WriteString(w, Banner)
}

// Status returns a callback that writes muted, indented progress lines to w.
func Status(w io.Writer) func(string) {
	return func(message string) {
		if w == nil {
			return
		}
		message = strings.TrimSpace(message)
		if message == "" {
			return
		}
		_, _ = io.WriteString(w, statusPrefix+message+statusSuffix)
	}
}
