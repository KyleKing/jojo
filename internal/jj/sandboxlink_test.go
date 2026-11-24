package jj_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/kyleking/jojo/internal/jj"
)

// linkVerifiedJJ is the newest jj the hard-linked sandbox has been checked against: every preview
// verb leaves the files it shares with the repository as they were. The check runs against whatever
// jj is installed, and a newer one fails this test until someone has read what it changed about the
// op store and the index and bumped this.
const linkVerifiedJJ = "0.47"

var jjVersionRE = regexp.MustCompile(`jj (\d+)\.(\d+)`)

// minor is a jj version's major.minor as one comparable number.
func minor(t *testing.T, version string) int {
	t.Helper()

	match := jjVersionRE.FindStringSubmatch("jj " + version)
	if match == nil {
		t.Fatalf("cannot read a jj version from %q", version)
	}

	var parts [2]int

	for i, text := range match[1:] {
		n, err := strconv.Atoi(text)
		if err != nil {
			t.Fatalf("cannot read a jj version from %q: %v", version, err)
		}

		parts[i] = n
	}

	return parts[0]*1000 + parts[1]
}

// Each verb the destination picker previews runs in the sandbox, and none of them writes through a
// hard link into the repository: jj adds its op store and index files beside the old ones and never
// rewrites one, which is the whole of what makes linking rather than copying safe.
func TestSandbox_EveryPreviewVerbLeavesTheLinkedFilesAlone(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)

	out, err := exec.CommandContext(t.Context(), "jj", "--version").Output()
	if err != nil {
		t.Fatalf("jj --version: %v", err)
	}

	installed := jjVersionRE.FindStringSubmatch(string(out))
	if installed == nil {
		t.Fatalf("cannot read a jj version from %q", out)
	}

	if version := installed[1] + "." + installed[2]; minor(t, version) > minor(t, linkVerifiedJJ) {
		t.Errorf("jj %s is newer than the %s the hard-linked preview sandbox was verified against: "+
			"check its op store and index still only add files, then bump linkVerifiedJJ", version, linkVerifiedJJ)
	}

	sandbox, err := client.OpenSandbox(t.Context())
	if err != nil {
		t.Fatalf("OpenSandbox: %v", err)
	}

	first, third := changeOf(t, dir, "description(exact:'first\n')"), changeOf(t, dir, "description(exact:'third\n')")
	place := jj.PlaceOptions{Destination: []string{first}}

	for name, args := range map[string][]string{
		"rebase":    jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{first}}),
		"squash":    jj.SquashIntoArgs(jj.SquashOptions{Sources: []string{third}, Into: first}),
		"duplicate": jj.DuplicateArgs([]string{third}, place),
		"new":       jj.NewArgs(place, true),
	} {
		_, err := sandbox.Preview(t.Context(), args, "all()", 0)
		if err != nil && !errors.Is(err, jj.ErrNothingChanged) {
			t.Errorf("previewing %s: %v", name, err)
		}

		if err := sandbox.Verify(); err != nil {
			t.Errorf("after previewing %s: %v", name, err)
		}
	}

	if err := sandbox.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// A linked file that changes is reported, by Verify and again by Close, so a jj that has started to
// rewrite its files in place is caught the first time a picker closes rather than never.
func TestSandbox_VerifyCatchesALinkedFileRewrittenInPlace(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)

	sandbox, err := client.OpenSandbox(t.Context())
	if err != nil {
		t.Fatalf("OpenSandbox: %v", err)
	}

	ops, err := filepath.Glob(filepath.Join(dir, ".jj", "repo", "op_store", "operations", "*"))
	if err != nil || len(ops) == 0 {
		t.Fatalf("no operation files to rewrite: %v", err)
	}

	// What an in-place write looks like through either name: the inode's content and time move.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(ops[0], later, later); err != nil {
		t.Fatal(err)
	}

	if err := sandbox.Verify(); !errors.Is(err, jj.ErrLinkRewritten) {
		t.Errorf("Verify() = %v after a linked file changed, want ErrLinkRewritten", err)
	}

	if err := sandbox.Close(); !errors.Is(err, jj.ErrLinkRewritten) {
		t.Errorf("Close() = %v after a linked file changed, want ErrLinkRewritten", err)
	}
}
