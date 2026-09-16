package jj

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// aliasName matches the quoted key of one `jj config list revset-aliases` line,
// which is the alias with its parameter list still attached.
var aliasName = regexp.MustCompile(`^revset-aliases\."([^"]+)"`)

// RevsetAliases returns the repository's revset aliases, defaults included, so
// completion offers trunk() and immutable_heads() alongside jj's own functions.
func (c *Client) RevsetAliases(ctx context.Context) ([]string, error) {
	out, err := c.executeJJ(ctx, "config", "list", "--include-defaults", "revset-aliases")
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}

	for line := range strings.SplitSeq(out, "\n") {
		match := aliasName.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		seen[strings.TrimSuffix(match[1], "()")] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)

	return names, nil
}
