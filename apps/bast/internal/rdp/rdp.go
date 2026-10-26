// Package rdp builds and launches a native Remote Desktop session.
// Username and domain are accepted on Session so they can be added later.
// They are omitted from the client command while empty, and the client prompts.
package rdp

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

// Session is one Remote Desktop target. User and Domain stay empty until a
// later change supplies them. Empty values are left out of the command.
type Session struct {
	Address string
	User    string
	Domain  string
}

var (
	errAddressEmpty      = errors.New("this host has no address to open in Remote Desktop")
	errAddressInvalid    = errors.New("that address can't be opened in Remote Desktop")
	errCredentialInvalid = errors.New("that Remote Desktop username or domain can't be used")

	lookPathFn     = exec.LookPath
	startProcessFn = startProcessDefault
	combinedFn     = combinedOutputDefault
)

// Command returns the native client and its argument vector for goos.
// The result is argv, never a shell string. User and Domain are appended
// to desktop URLs only when set. mstsc has no equivalent flag, so Windows
// keeps /v: and the client prompts.
func Command(goos string, session Session) (string, []string, error) {
	address, err := canonicalAddress(session.Address)
	if err != nil {
		return "", nil, err
	}
	user, err := canonicalCredential(session.User)
	if err != nil {
		return "", nil, err
	}
	domain, err := canonicalCredential(session.Domain)
	if err != nil {
		return "", nil, err
	}
	switch goos {
	case "windows":
		return "mstsc", []string{"/v:" + address}, nil
	case "darwin":
		return "open", []string{desktopURL(address, user, domain, true)}, nil
	default:
		return "xdg-open", []string{desktopURL(address, user, domain, false)}, nil
	}
}

// Launch starts the native client for this operating system.
// Windows detaches mstsc because it stays open. macOS and Linux wait for
// open or xdg-open, which exit immediately and report a missing handler.
func Launch(session Session) error {
	return launch(runtime.GOOS, session)
}

func launch(goos string, session Session) error {
	name, args, err := Command(goos, session)
	if err != nil {
		return err
	}
	if goos == "windows" {
		if _, err := lookPathFn(name); err != nil {
			return missingClientError(goos)
		}
		if err := startProcessFn(name, args...); err != nil {
			return launchError(goos, err)
		}
		return nil
	}
	if _, err := combinedFn(name, args...); err != nil {
		return launchError(goos, err)
	}
	return nil
}

func startProcessDefault(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func combinedOutputDefault(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func desktopURL(address, user, domain string, fullAddress bool) string {
	// The short rdp://host form has nowhere to put credentials. Any user or
	// domain switches Linux onto the same full-address URL macOS always uses.
	if !fullAddress && user == "" && domain == "" {
		return "rdp://" + address
	}
	target := "rdp://full%20address=s:" + address
	if user != "" {
		target += "&username=s:" + escapeRDPValue(user)
	}
	if domain != "" {
		target += "&domain=s:" + escapeRDPValue(domain)
	}
	return target
}

func escapeRDPValue(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func canonicalAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errAddressEmpty
	}
	if strings.IndexFunc(raw, func(r rune) bool {
		return r < 0x20 || r == 0x7f || unicode.IsSpace(r)
	}) >= 0 || strings.HasPrefix(raw, "-") {
		return "", errAddressInvalid
	}
	host, port, err := splitAddress(raw)
	if err != nil {
		return "", err
	}
	canonicalHost, err := canonicalHost(host)
	if err != nil {
		return "", err
	}
	canonicalPort, err := canonicalPort(port)
	if err != nil {
		return "", err
	}
	if canonicalPort == "" {
		return canonicalHost, nil
	}
	return canonicalHost + ":" + canonicalPort, nil
}

func splitAddress(raw string) (string, string, error) {
	if strings.HasPrefix(raw, "[") {
		end := strings.IndexByte(raw, ']')
		if end <= 1 {
			return "", "", errAddressInvalid
		}
		host := raw[1:end]
		rest := raw[end+1:]
		if rest == "" {
			return host, "", nil
		}
		if len(rest) < 2 || rest[0] != ':' || strings.Contains(rest[1:], ":") {
			return "", "", errAddressInvalid
		}
		return host, rest[1:], nil
	}
	if ip := net.ParseIP(raw); ip != nil && ip.To4() == nil {
		return raw, "", nil
	}
	host, port, found := strings.Cut(raw, ":")
	if !found {
		return raw, "", nil
	}
	if host == "" || strings.Contains(port, ":") {
		return "", "", errAddressInvalid
	}
	return host, port, nil
}

func canonicalHost(host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			return ip.String(), nil
		}
		return "[" + ip.String() + "]", nil
	}
	if !validHostname(host) {
		return "", errAddressInvalid
	}
	return strings.TrimSuffix(host, "."), nil
}

func validHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if strings.HasSuffix(host, ".") {
		host = strings.TrimSuffix(host, ".")
		if host == "" || len(host) > 253 {
			return false
		}
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			default:
				return false
			}
		}
	}
	return true
}

func canonicalPort(port string) (string, error) {
	if port == "" {
		return "", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return "", errAddressInvalid
	}
	return port, nil
}

func canonicalCredential(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 128 || strings.HasPrefix(value, "-") {
		return "", errCredentialInvalid
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-', r == '@':
		default:
			return "", errCredentialInvalid
		}
	}
	return value, nil
}

func missingClientError(goos string) error {
	switch goos {
	case "windows":
		return errors.New("Remote Desktop isn't available. Install the Windows Remote Desktop client (mstsc) and try again.")
	case "darwin":
		return errors.New("Couldn't open Remote Desktop. Install Microsoft Remote Desktop and try again.")
	default:
		return errors.New("Couldn't open Remote Desktop. Install an RDP client and xdg-utils, then try again.")
	}
}

func launchError(goos string, err error) error {
	if err == nil {
		return nil
	}
	if goos != "windows" || isMissingExecutable(err) {
		return missingClientError(goos)
	}
	return errors.New("Couldn't start Remote Desktop. Check that the Windows Remote Desktop client can open, then try again.")
}

func isMissingExecutable(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file")
}
