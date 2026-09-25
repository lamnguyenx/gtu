package gitignore

import (
	"fmt"
	"regexp"
	"strings"
)

// compilePattern translates a single gitignore line into a Pattern whose regex
// matches paths relative to the .gitignore file's directory.
func compilePattern(line string) (*Pattern, error) {
	p := &Pattern{}

	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = line[1:]
	}

	line = strings.TrimPrefix(line, "\\")

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = line[:len(line)-1]
	}

	if line == "" {
		return nil, fmt.Errorf("empty pattern")
	}

	re, err := buildRegex(line)
	if err != nil {
		return nil, err
	}
	p.regex = re
	return p, nil
}

// buildRegex translates a gitignore glob into a Go regexp that matches a
// forward-slash-normalized relative path.
//
// Anchoring rules (per Git spec):
//   - leading /  → anchored to the .gitignore directory
//   - internal / → anchored to the .gitignore directory
//   - no /       → matches at any depth
func buildRegex(pattern string) (*regexp.Regexp, error) {
	anchored := strings.Contains(pattern, "/")

	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
		anchored = true
	}

	var sb strings.Builder

	if anchored {
		sb.WriteString("^")
	} else {
		sb.WriteString("(?:^|.*/)")
	}

	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i += 2
				switch {
				case i < len(pattern) && pattern[i] == '/':
					sb.WriteString("(?:.*/)?")
					i++
				case i >= len(pattern):
					sb.WriteString(".*")
				default:
					sb.WriteString(".*")
				}
			} else {
				sb.WriteString("[^/]*")
				i++
			}
		case '?':
			sb.WriteString("[^/]")
			i++
		case '[':
			i = writeCharClass(&sb, pattern, i)
		case '.':
			sb.WriteString(`\.`)
			i++
		case '/', '+', '(', ')', '^', '$', '{', '}', '|', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}

	sb.WriteString("(?:/.*)?$")

	return regexp.Compile(sb.String())
}

// writeCharClass translates a glob character class beginning at pattern[i]
// (which must be '[') into an equivalent regexp fragment and returns the index
// just past the closing bracket.
func writeCharClass(sb *strings.Builder, pattern string, i int) int {
	j := i + 1
	if j < len(pattern) && (pattern[j] == '!' || pattern[j] == '^') {
		j++
	}
	if j < len(pattern) && pattern[j] == ']' {
		j++
	}
	for j < len(pattern) && pattern[j] != ']' {
		j++
	}
	if j < len(pattern) {
		content := pattern[i+1 : j]
		if strings.HasPrefix(content, "!") {
			content = "^" + content[1:]
		}
		sb.WriteString("[" + content + "]")
		return j + 1
	}
	sb.WriteString(`\[`)
	return i + 1
}
