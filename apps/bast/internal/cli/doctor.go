package cli

import (
	"context"
	"time"

	"bast/internal/doctor"
	"bast/internal/telemetry"
)

func (r *Runner) doctor(args []string) error {
	fs := newFlagSet("doctor")
	fix := fs.Bool("fix", false, "")
	probe := fs.Bool("probe", false, "")
	var categories stringsFlag
	fs.Var(&categories, "category", "")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast doctor [--fix] [--probe] [--category name]")
	}
	for _, c := range categories {
		if !doctor.ValidCategory(c) {
			return usagef("unknown doctor category %q", c)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	eng := doctor.New(r.Paths, r.OpenSSH, r.Version)
	report := eng.Run(ctx, doctor.Options{Fix: *fix, Probe: *probe, Categories: []string(categories)})
	telemetry.Track("doctor", r.Version)
	if r.JSON {
		if err := r.success(report, ""); err != nil {
			return err
		}
	} else if err := doctor.Format(r.Out, report); err != nil {
		return err
	}
	if report.HasFail() {
		return silentExit{code: 1}
	}
	return nil
}
