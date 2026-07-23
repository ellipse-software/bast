package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"
)

const defaultEndpoint = "https://bast.sh/api/telemetry"

var endpoint = defaultEndpoint

type payload struct {
	Event   string `json:"event"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Source  string `json:"source"`
}

func Enabled() bool {
	return os.Getenv("BAST_NO_TELEMETRY") == ""
}

func Track(event, version string) {
	if !Enabled() {
		return
	}

	go send(event, version)
}

func send(event, version string) {
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

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
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
