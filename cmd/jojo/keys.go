package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/kyleking/aragonite/keyusage"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/model"
)

const keysCommand = "keys"

// cliRoot is the command tree usage is recorded and reported against: the flags declared on fs and
// the one subcommand.
func cliRoot(fs *flag.FlagSet) keyusage.Command {
	var flags []string

	fs.VisitAll(func(f *flag.Flag) {
		flags = append(flags, flagSpelling(f.Name))
	})

	return keyusage.Command{
		Name:  "jojo",
		What:  "browse a Jujutsu repository",
		Flags: flags,
		Subs:  []keyusage.Command{{Name: keysCommand, What: "report key usage"}},
	}
}

func flagSpelling(name string) string {
	if len(name) == 1 {
		return "-" + name
	}

	return "--" + name
}

// loadSeen is what earlier sessions logged. It is nil when recording is off or the log cannot be
// read, so a legend brightens nothing.
func loadSeen(rec *keyusage.Recorder, keys help.Keymap) *keyusage.Usage {
	if rec == nil {
		return nil
	}

	u, err := keyusage.Load(keyusage.Dir(model.UsageApp), model.UsageKeymap(keys))
	if err != nil {
		return nil
	}

	return &u
}

// keysReport reads the usage log back against the keymap.
func keysReport(stdout io.Writer, keys help.Keymap) error {
	dir := keyusage.Dir(model.UsageApp)
	if dir == "" {
		_, err := fmt.Fprintf(stdout, "no usage log: set %s or create ~/Sync\n", keyusage.EnvVar(model.UsageApp))

		return err //nolint:wrapcheck // a failed write to stdout has nothing to add
	}

	km := model.UsageKeymap(keys)
	km[keyusage.CLI] = keyusage.Commands(cliRoot(flag.CommandLine))

	u, err := keyusage.Load(dir, km)
	if err != nil {
		return fmt.Errorf("reading the usage log: %w", err)
	}

	if err := keyusage.Write(stdout, u, km, time.Now().Add(-model.StaleAfter)); err != nil {
		return fmt.Errorf("writing the key report: %w", err)
	}

	return nil
}
