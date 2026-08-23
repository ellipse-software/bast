package doctor

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"bast/internal/keys"
)

func (e Engine) checkKeys(ctx context.Context, r *Report, st runState) {
	for _, key := range st.keys {
		if len(key.References) > 0 && key.PrivatePath != "" && !fileExists(key.PrivatePath) {
			r.add(Finding{
				ID: "keys.referenced_missing", Severity: SeverityFail, Category: CatKeys,
				Title: "Referenced key \"" + key.Name + "\" is missing", Path: e.display(key.PrivatePath),
				Detail: "Hosts " + strings.Join(key.References, ", ") + " point at this identity.",
			})
		}
		if key.PrivatePath != "" && key.PublicPath == "" {
			r.add(Finding{
				ID: "keys.private_without_public", Severity: SeverityWarn, Category: CatKeys,
				Title: "Key \"" + key.Name + "\" has no .pub file", Path: e.display(key.PrivatePath),
			})
		}
		if key.PublicPath != "" && key.PrivatePath == "" && !key.InAgent {
			r.add(Finding{
				ID: "keys.public_without_private", Severity: SeverityWarn, Category: CatKeys,
				Title: "Key \"" + key.Name + "\" has a public file only", Path: e.display(key.PublicPath),
			})
		}
		if len(key.References) == 0 && key.Managed && key.PrivatePath != "" {
			r.add(Finding{
				ID: "keys.unreferenced", Severity: SeverityInfo, Category: CatKeys,
				Title: "Key \"" + key.Name + "\" is not used by any host", Path: e.display(key.PrivatePath),
			})
		}
		algo := strings.ToLower(key.Algorithm)
		if algo == "dsa" || algo == "ssh-dss" {
			r.add(Finding{
				ID: "keys.dsa", Severity: SeverityWarn, Category: CatKeys,
				Title: "Key \"" + key.Name + "\" uses DSA", Path: e.display(key.PrivatePath),
				Detail: "OpenSSH has disabled DSA by default.",
			})
		}
		if algo == "rsa" || algo == "ssh-rsa" {
			if bits := e.keyBits(ctx, key); bits > 0 && bits < 2048 {
				r.add(Finding{
					ID: "keys.rsa_small", Severity: SeverityWarn, Category: CatKeys,
					Title: "Key \"" + key.Name + "\" is RSA shorter than 2048 bits", Path: e.display(key.PrivatePath),
				})
			}
		}
		if key.PrivatePath != "" && !key.InAgent && e.keyEncrypted(ctx, key.PrivatePath) {
			r.add(Finding{
				ID: "keys.passphrase_not_in_agent", Severity: SeverityWarn, Category: CatKeys,
				Title:  "Passphrase-protected key \"" + key.Name + "\" is not in the agent",
				Path:   e.display(key.PrivatePath),
				Detail: "Each connection will prompt for the passphrase.",
				Fix:    "ssh-add " + e.display(key.PrivatePath),
			})
		}
	}
}

func (e Engine) keyBits(ctx context.Context, key keys.Key) int {
	inspect := key.PublicPath
	if inspect == "" {
		inspect = key.PrivatePath
	}
	if inspect == "" {
		return 0
	}
	bin := e.OpenSSH.SSHKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "-lf", inspect)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}
	return n
}

func (e Engine) keyEncrypted(ctx context.Context, path string) bool {
	if path == "" {
		return false
	}
	if b, err := os.ReadFile(path); err == nil {
		s := string(b)
		if strings.Contains(s, "ENCRYPTED") || strings.Contains(s, "bcrypt") {
			return true
		}
	}
	bin := e.OpenSSH.SSHKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "-y", "-P", "", "-f", path)
	out, err := cmd.CombinedOutput()
	if err == nil || cctx.Err() != nil {
		return false
	}
	msg := strings.ToLower(string(out) + err.Error())
	return strings.Contains(msg, "passphrase") || strings.Contains(msg, "decrypt")
}
