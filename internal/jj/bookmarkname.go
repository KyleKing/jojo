package jj

import (
	"errors"
	"strings"
	"unicode"
)

// ErrBookmarkName marks a bookmark name jj would refuse, or would keep but could not export to git.
var ErrBookmarkName = errors.New("not a bookmark name")

// nameError is why a name was refused, said on one line for the prompt's footer, and is
// ErrBookmarkName to errors.Is.
type nameError string

func (e nameError) Error() string { return string(e) }

func (nameError) Is(target error) bool { return target == ErrBookmarkName }

// CheckBookmarkName answers why name cannot be a bookmark, or nil when it can. A name is read as a
// revset symbol, so it takes letters, digits, underscores, hyphens, slashes, dots, and pluses, and
// nothing else unquoted. Git then refuses a ref that starts with a dot or a slash, ends with one or
// with .lock, or holds two dots or two slashes in a row, which jj keeps but cannot export. The
// prompt asks before jj does, so the name is fixed in place.
func CheckBookmarkName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return nameError("a bookmark needs a name")
	case strings.ContainsFunc(name, unicode.IsSpace):
		return nameError("a bookmark name has no spaces")
	case strings.ContainsFunc(name, notSymbol):
		return nameError("a bookmark name is letters, digits, and _ - / . +")
	case strings.HasPrefix(name, "-"):
		return nameError("a bookmark name cannot start with -")
	case badRef(name):
		return nameError("git cannot hold that name: no .. or //, no part " +
			"starting with a dot, and no / . or .lock at the end")
	}

	return nil
}

// notSymbol reports a rune jj does not read as part of a bare revset symbol.
func notSymbol(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-/.+", r)
}

// badRef reports a name git refuses as a ref.
func badRef(name string) bool {
	if strings.Contains(name, "..") || strings.Contains(name, "//") ||
		strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") {
		return true
	}

	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || part == "" {
			return true
		}
	}

	return false
}
