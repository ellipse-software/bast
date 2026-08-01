package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"bast/internal/telemetry"
	"bast/internal/updater"
)

func (r *Runner) update(args []string) error {
	if len(args) != 0 {
		return usagef("usage: bast update")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	stdout, stderr := r.Out, r.Err
	if r.JSON {
		stdout, stderr = io.Discard, io.Discard
	}
	client := &http.Client{Timeout: 10 * time.Second}
	if err := updater.Update(context.Background(), client, executable, stdout, stderr); err != nil {
		return err
	}
	telemetry.Track("update", r.Version)
	return r.success(map[string]string{"status": "updated"}, "")
}
