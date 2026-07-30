package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
)

const (
	defaultEndpoint      = "https://bast.sh/api/telemetry"
	defaultErrorEndpoint = "https://bast.sh/api/errors"
)

var (
	endpoint      = defaultEndpoint
	errorEndpoint = defaultErrorEndpoint
	endpointMu    sync.RWMutex
)

// SetErrorEndpoint overrides the error-report destination and returns a restore function.
func SetErrorEndpoint(url string) func() {
	endpointMu.Lock()
	prev := errorEndpoint
	errorEndpoint = url
	endpointMu.Unlock()
	return func() {
		endpointMu.Lock()
		errorEndpoint = prev
		endpointMu.Unlock()
	}
}

func setEndpoint(url string) func() {
	endpointMu.Lock()
	prev := endpoint
	endpoint = url
	endpointMu.Unlock()
	return func() {
		endpointMu.Lock()
		endpoint = prev
		endpointMu.Unlock()
	}
}

type payload struct {
	Event   string `json:"event"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Source  string `json:"source"`
}

// Report contains a CLI or TUI error report.
type Report struct {
	Message string `json:"message"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Source  string `json:"source"`
	Code    string `json:"code,omitempty"`
	Stack   string `json:"stack,omitempty"`
	Context string `json:"context,omitempty"`
	Command string `json:"command,omitempty"`
}

const (
	ReportPrompt = "\r\n\x1b[38;2;107;114;128m Press Space to send an error report (may include error text and stack traces), any other key to continue.\x1b[0m\r\n"
)

func Enabled() bool {
	return os.Getenv("BAST_NO_TELEMETRY") == ""
}

func Track(event, version string) {
	if !Enabled() {
		return
	}

	endpointMu.RLock()
	target := endpoint
	endpointMu.RUnlock()
	go send(target, event, version)
}

// ReportError sends an error report synchronously so CLI callers can wait for it to flush.
func ReportError(r Report) error {
	if !Enabled() {
		return nil
	}
	if r.Message == "" {
		return nil
	}
	if r.OS == "" {
		r.OS = platformOS()
	}
	if r.Arch == "" {
		r.Arch = platformArch()
	}
	if r.Source == "" {
		r.Source = "cli"
	}
	endpointMu.RLock()
	target := errorEndpoint
	endpointMu.RUnlock()
	return sendError(target, r)
}

// OfferReport prompts for consent and sends the report when accepted.
func OfferReport(in io.Reader, out io.Writer, r Report) {
	if !Enabled() {
		return
	}
	if out != nil {
		_, _ = io.WriteString(out, ReportPrompt)
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
	n, _ := in.Read(buf[:])
	if n == 0 {
		return
	}
	if buf[0] == ' ' {
		_ = ReportError(r)
	}
}

func send(target, event, version string) {
	body, err := json.Marshal(payload{
		Event:   event,
		Version: version,
		OS:      platformOS(),
		Arch:    platformArch(),
		Source:  "cli",
	})
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func sendError(target string, r Report) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("error report failed with status %d", resp.StatusCode)
	}
	return nil
}

func platformOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

func platformArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "amd64":
		return "amd64"
	default:
		return runtime.GOARCH
	}
}
