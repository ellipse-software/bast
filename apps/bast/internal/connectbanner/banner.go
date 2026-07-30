package connectbanner

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

const Banner = "\x1b[1;38;2;139;92;246m BAST \x1b[0m  Connecting to server…\r\n" +
	"\x1b[38;2;107;114;128m Stuck? Press Enter, then ~. to return to Bast.\x1b[0m\r\n\r\n"

const ContinuePrompt = "\r\n\x1b[38;2;107;114;128m Press any key to continue back to Bast.\x1b[0m\r\n"

const (
	statusPrefix = "\x1b[38;2;107;114;128m "
	statusSuffix = "\x1b[0m\r\n"
)

func Write(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = io.WriteString(w, Banner)
}

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

// If in is a terminal, it briefly enters raw mode so any key (not only Enter) works.
// If in is not a terminal, it still attempts a single-byte read so tests can drive it.
func WaitToContinue(in io.Reader, out io.Writer) {
	if out != nil {
		_, _ = io.WriteString(out, ContinuePrompt)
	}
	if in == nil {
		return
	}

	file, ok := in.(*os.File)
	if ok && term.IsTerminal(file.Fd()) {
		state, err := term.MakeRaw(file.Fd())
		if err == nil {
			defer func() { _ = term.Restore(file.Fd(), state) }()
		}
	}

	var buf [1]byte
	_, _ = in.Read(buf[:])
}
