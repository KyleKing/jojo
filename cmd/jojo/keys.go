package main

import (
	"fmt"
	"io"
	"time"

	"github.com/kyleking/aragonite/keyusage"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/model"
)

// openUsage is the recorder for this session and what earlier sessions logged. Both are nil when
// recording is off, and seen is nil when the log cannot be read, so a legend brightens nothing.
func openUsage(keys help.Keymap) (*keyusage.Recorder, *keyusage.Usage) {
	rec := keyusage.Open(model.UsageApp)
	if rec == nil {
		return nil, nil
	}

	u, err := keyusage.Load(keyusage.Dir(model.UsageApp), model.UsageKeymap(keys))
	if err != nil {
		return rec, nil
	}

	return rec, &u
}

// keysReport reads the usage log back against the keymap.
func keysReport(stdout io.Writer, keys help.Keymap) error {
	dir := keyusage.Dir(model.UsageApp)
	if dir == "" {
		_, err := fmt.Fprintf(stdout, "no usage log: set %s or create ~/Sync\n", keyusage.EnvVar(model.UsageApp))

		return err //nolint:wrapcheck // a failed write to stdout has nothing to add
	}

	km := model.UsageKeymap(keys)

	u, err := keyusage.Load(dir, km)
	if err != nil {
		return fmt.Errorf("reading the usage log: %w", err)
	}

	if err := keyusage.Write(stdout, u, km, time.Now().Add(-model.StaleAfter)); err != nil {
		return fmt.Errorf("writing the key report: %w", err)
	}

	return nil
}
