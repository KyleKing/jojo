package revsetbox

import "unicode"

// isIdentRune reports whether r can appear in a revset function or alias identifier.
func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// scan walks runes[:upTo], tracking quote state and the stack of function names whose
// call parens are still open at that point. A backslash inside a quote escapes the next
// rune, so an escaped quote character does not end the string early.
func scan(runes []rune, upTo int) (rune, []string) {
	if upTo > len(runes) {
		upTo = len(runes)
	}

	var (
		quote rune
		stack []string
	)

	for i := 0; i < upTo; i++ {
		r := runes[i]

		switch {
		case quote != 0:
			if r == '\\' {
				i++

				continue
			}

			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(':
			end := i
			start := end
			for start > 0 && isIdentRune(runes[start-1]) {
				start--
			}

			stack = append(stack, string(runes[start:end]))
		case r == ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	return quote, stack
}

// Balanced reports whether every paren and quote in s is closed.
func Balanced(s string) bool {
	runes := []rune(s)
	quote, stack := scan(runes, len(runes))

	return quote == 0 && len(stack) == 0
}

// inQuote reports whether pos sits inside an unterminated quoted string.
func inQuote(runes []rune, pos int) bool {
	quote, _ := scan(runes, pos)

	return quote != 0
}

// enclosingCall returns the name of the function whose call parens are still open at
// pos, if any.
func enclosingCall(runes []rune, pos int) (string, bool) {
	_, stack := scan(runes, pos)
	if len(stack) == 0 {
		return "", false
	}

	name := stack[len(stack)-1]

	return name, name != ""
}

// identifierRange returns the bounds of the identifier touching pos, as an empty range
// when pos sits between two non-identifier runes.
func identifierRange(runes []rune, pos int) (int, int) {
	if pos < 0 {
		pos = 0
	}

	if pos > len(runes) {
		pos = len(runes)
	}

	start, end := pos, pos
	for start > 0 && isIdentRune(runes[start-1]) {
		start--
	}

	for end < len(runes) && isIdentRune(runes[end]) {
		end++
	}

	return start, end
}
