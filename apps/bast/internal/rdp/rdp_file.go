package rdp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FileSession is one Remote Desktop file. Password is not a field.
type FileSession struct {
	Address    string
	Port       string
	User       string
	Domain     string
	Fullscreen bool
	Admin      bool
}

// ExecCommand starts the system client. Tests replace it.
var ExecCommand = exec.Command

// RenderFile returns an .rdp document. It never contains a password line.
func RenderFile(session FileSession) (string, error) {
	target, err := DialTarget(session.Address, session.Port)
	if err != nil {
		return "", err
	}
	host, port := splitFileTarget(target)
	user, domain, err := SplitSignIn(session.User, session.Domain)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "full address:s:%s\r\n", host)
	if port != "" {
		fmt.Fprintf(&b, "server port:i:%s\r\n", port)
	}
	if user != "" {
		fmt.Fprintf(&b, "username:s:%s\r\n", user)
	}
	if domain != "" {
		fmt.Fprintf(&b, "domain:s:%s\r\n", domain)
	}
	b.WriteString("prompt for credentials:i:1\r\n")
	if session.Admin {
		b.WriteString("administrative session:i:1\r\n")
	}
	if session.Fullscreen {
		b.WriteString("screen mode id:i:2\r\n")
	}
	return b.String(), nil
}

// LaunchFile writes a temporary .rdp file and opens it in the system client.
func LaunchFile(session FileSession) error {
	_, err := StartFile(session)
	return err
}

// StartFile is LaunchFile. On Windows the returned command is the running
// mstsc process. On other systems the opener has already exited and the
// command is nil.
func StartFile(session FileSession) (*exec.Cmd, error) {
	body, err := RenderFile(session)
	if err != nil {
		return nil, err
	}
	path, err := writeRDPFile(body)
	if err != nil {
		return nil, err
	}
	return startRDPFile(runtime.GOOS, path)
}

func writeRDPFile(body string) (string, error) {
	dir := filepath.Join(os.TempDir(), "bast-rdp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "session-*.rdp")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if _, err := file.WriteString(body); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func startRDPFile(goos, path string) (*exec.Cmd, error) {
	name, args, detach := fileCommand(goos, path)
	cmd := ExecCommand(name, args...)
	if detach {
		if _, err := lookPathFn(name); err != nil {
			return nil, missingClientError(goos)
		}
		if err := cmd.Start(); err != nil {
			return nil, launchError(goos, err)
		}
		return cmd, nil
	}
	if _, err := cmd.CombinedOutput(); err != nil {
		if goos != "darwin" && goos != "windows" {
			if freerdp, lookErr := lookPathFn("xfreerdp"); lookErr == nil {
				fallback := ExecCommand(freerdp, path)
				if err := fallback.Start(); err == nil {
					go func() { _ = fallback.Wait() }()
					return nil, nil
				}
			}
		}
		return nil, missingClientError(goos)
	}
	return nil, nil
}

func fileCommand(goos, path string) (string, []string, bool) {
	switch goos {
	case "windows":
		return "mstsc", []string{path}, true
	case "darwin":
		return "open", []string{path}, false
	default:
		return "xdg-open", []string{path}, false
	}
}

func splitFileTarget(target string) (string, string) {
	if strings.HasPrefix(target, "[") {
		end := strings.IndexByte(target, ']')
		if end > 1 {
			host := target[1:end]
			rest := target[end+1:]
			if strings.HasPrefix(rest, ":") {
				return host, rest[1:]
			}
			return host, ""
		}
	}
	host, port, found := strings.Cut(target, ":")
	if !found {
		return target, ""
	}
	return host, port
}
