package revsetbox

import (
	"sort"
	"strings"
)

// candidates returns the names, from the built-in function table and aliases, that
// start with prefix, deduplicated and sorted. An empty prefix matches nothing, since a
// completion popup for every identifier the user hasn't started typing is just noise.
func candidates(prefix string, aliases []string) []string {
	if prefix == "" {
		return nil
	}

	seen := make(map[string]bool, len(builtinNames))
	matches := make([]string, 0, len(builtinNames))

	for _, name := range builtinNames {
		if strings.HasPrefix(name, prefix) {
			seen[name] = true
			matches = append(matches, name)
		}
	}

	for _, name := range aliases {
		if strings.HasPrefix(name, prefix) && !seen[name] {
			seen[name] = true
			matches = append(matches, name)
		}
	}

	sort.Strings(matches)

	return matches
}

// signatureHelp returns the one-line signature and blurb for name, checking the
// built-in table first and falling back to a generic alias description.
func signatureHelp(name string, aliases []string) (string, bool) {
	if spec, ok := builtinByName[name]; ok {
		return spec.Signature + "  " + spec.Blurb, true
	}

	for _, alias := range aliases {
		if alias == name {
			return name + "()  repo revset alias", true
		}
	}

	return "", false
}
