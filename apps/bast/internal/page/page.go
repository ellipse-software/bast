// Package page opens an http or https URL in a browser.
// A tunnelled page uses a separate browser profile and a SOCKS proxy so the
// system proxy and the user's normal profile stay untouched.
package page

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errURLHost     = errors.New("that URL needs a host")
	errURLScheme   = errors.New("that URL needs http or https")
	errURLUserinfo = errors.New("that URL can't include a username or password")
	errURLInvalid  = errors.New("that URL can't be used")
	errNoBrowser   = errors.New("install Chrome, Edge, Brave, or Firefox to open a tunnelled page")
)

// ExecCommand starts a browser. Tests replace it.
var ExecCommand = exec.Command

type Browser struct {
	Name string
	Path string
}

// ParseURL accepts http and https URLs with a host and no userinfo.
func ParseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\x00 \t") {
		return "", errURLInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return "", errURLHost
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errURLScheme
	}
	if parsed.User != nil {
		return "", errURLUserinfo
	}
	parsed.Scheme = scheme
	return parsed.String(), nil
}

// Open starts the system browser for a direct URL.
func Open(raw string) error {
	parsed, err := ParseURL(raw)
	if err != nil {
		return err
	}
	name, args := SystemOpen(runtime.GOOS, parsed)
	cmd := ExecCommand(name, args...)
	return cmd.Start()
}

func SystemOpen(goos, raw string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{raw}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", raw}
	default:
		return "xdg-open", []string{raw}
	}
}

// OpenProxied starts a browser window that sends raw through a loopback SOCKS proxy.
// The profile directory is reused for the same jump alias.
func OpenProxied(cacheDir, via string, port int, raw string) error {
	parsed, err := ParseURL(raw)
	if err != nil {
		return err
	}
	if port < 1 || port > 65535 {
		return errors.New("that tunnel port can't be used")
	}
	browser, err := FindBrowser(runtime.GOOS)
	if err != nil {
		return err
	}
	profile := filepath.Join(cacheDir, "web-profiles", profileKey(via))
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	name, args, prefs := LaunchSpec(browser, port, profile, parsed)
	if prefs != "" {
		if err := os.WriteFile(filepath.Join(profile, "user.js"), []byte(prefs), 0o600); err != nil {
			return err
		}
	}
	cmd := ExecCommand(name, args...)
	return cmd.Start()
}

func profileKey(via string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(via)))
	return hex.EncodeToString(sum[:8])
}

// LaunchSpec returns the browser argv. prefs is Firefox user.js content.
func LaunchSpec(browser Browser, port int, profile, raw string) (string, []string, string) {
	if browser.Name == "firefox" {
		return browser.Path, []string{
			"-no-remote",
			"-profile", profile,
			"-new-window", raw,
		}, firefoxPrefs(port)
	}
	return browser.Path, []string{
		"--user-data-dir=" + profile,
		fmt.Sprintf("--proxy-server=socks5://127.0.0.1:%d", port),
		`--host-resolver-rules=MAP * ~NOTFOUND , EXCLUDE 127.0.0.1`,
		"--no-first-run",
		"--new-window",
		raw,
	}, ""
}

func firefoxPrefs(port int) string {
	return fmt.Sprintf(`user_pref("network.proxy.type", 1);
user_pref("network.proxy.socks", "127.0.0.1");
user_pref("network.proxy.socks_port", %d);
user_pref("network.proxy.socks_version", 5);
user_pref("network.proxy.socks_remote_dns", true);
user_pref("network.proxy.no_proxies_on", "");
`, port)
}

// FindBrowser returns the first installed Chromium-family browser, then Firefox.
func FindBrowser(goos string) (Browser, error) {
	for _, candidate := range DefaultCandidates(goos) {
		if candidate.Path == "" {
			continue
		}
		info, err := os.Stat(candidate.Path)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return Browser{}, errNoBrowser
}

func DefaultCandidates(goos string) []Browser {
	names := []struct {
		name string
		bins []string
		mac  string
		win  []string
	}{
		{name: "chrome", bins: []string{"google-chrome", "google-chrome-stable", "chrome"}, mac: "Google Chrome.app/Contents/MacOS/Google Chrome", win: []string{filepath.Join("Google", "Chrome", "Application", "chrome.exe")}},
		{name: "edge", bins: []string{"microsoft-edge", "microsoft-edge-stable", "msedge"}, mac: "Microsoft Edge.app/Contents/MacOS/Microsoft Edge", win: []string{filepath.Join("Microsoft", "Edge", "Application", "msedge.exe")}},
		{name: "brave", bins: []string{"brave-browser", "brave"}, mac: "Brave Browser.app/Contents/MacOS/Brave Browser", win: []string{filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe")}},
		{name: "chromium", bins: []string{"chromium", "chromium-browser"}, mac: "Chromium.app/Contents/MacOS/Chromium", win: []string{filepath.Join("Chromium", "Application", "chrome.exe")}},
		{name: "firefox", bins: []string{"firefox"}, mac: "Firefox.app/Contents/MacOS/firefox", win: []string{filepath.Join("Mozilla Firefox", "firefox.exe")}},
	}
	var out []Browser
	for _, item := range names {
		switch goos {
		case "darwin":
			out = append(out, Browser{Name: item.name, Path: filepath.Join("/Applications", item.mac)})
		case "windows":
			roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")}
			for _, root := range roots {
				if root == "" {
					continue
				}
				for _, rel := range item.win {
					out = append(out, Browser{Name: item.name, Path: filepath.Join(root, rel)})
				}
			}
		}
		for _, bin := range item.bins {
			if path, err := exec.LookPath(bin); err == nil {
				out = append(out, Browser{Name: item.name, Path: path})
			}
		}
	}
	return out
}
