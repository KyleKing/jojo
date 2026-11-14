package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/keyusage"
)

func TestInvokeLogsFlagNamesNotValues(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("jojo", flag.ContinueOnError)
	fs.String("repo", ".", "")

	dir := t.TempDir()
	rec := keyusage.Open("jojo", keyusage.WithDir(dir))
	rec.Invoke(cliRoot(fs), []string{"keys", "--repo", "/secret/path"})

	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("log files = %v, %v", files, err)
	}

	logged, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(logged), "jojo keys --repo") || strings.Contains(string(logged), "/secret/path") {
		t.Fatalf("log = %s", logged)
	}
}
