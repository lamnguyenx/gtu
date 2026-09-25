// Package gitignore provides .gitignore-style pattern matching that supports
// per-directory discovery and stacking of rules, matching real Git semantics.
package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Pattern represents one compiled gitignore line.
type Pattern struct {
	regex   *regexp.Regexp
	negate  bool
	dirOnly bool
}

// Matcher holds all patterns from a single .gitignore file, relative to
// baseDir. Patterns are evaluated last-match-wins.
type Matcher struct {
	baseDir  string
	patterns []Pattern
}

// Stack is an ordered list of Matchers, from outermost ancestor to nearest.
// It is immutable: Push returns a new Stack.
type Stack []*Matcher

// LoadFromFile reads a .gitignore-style file and compiles its patterns.
// baseDir is the directory the patterns are anchored to.
func LoadFromFile(baseDir, filePath string) (*Matcher, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &Matcher{baseDir: filepath.Clean(baseDir)}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, err := compilePattern(line)
		if err != nil {
			continue
		}
		m.patterns = append(m.patterns, *p)
	}
	return m, scanner.Err()
}

// LoadFromDir checks dir for a .gitignore file and loads it if present.
// Returns (nil, nil) when no .gitignore exists.
func LoadFromDir(dir string) (*Matcher, error) {
	giPath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(giPath); err != nil {
		return nil, nil
	}
	return LoadFromFile(dir, giPath)
}

// LoadGitExclude loads the info/exclude file from the git metadata directory,
// handling both regular .git/ dirs and submodule .git files (gitdir: pointers).
func LoadGitExclude(dir string) (*Matcher, error) {
	gitDir := resolveGitDir(dir)
	if gitDir == "" {
		return nil, nil
	}
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if _, err := os.Stat(excludePath); err != nil {
		return nil, nil
	}
	return LoadFromFile(dir, excludePath)
}

// MatchWithNegation returns whether relPath matched and whether the last
// matching pattern was a negation.
func (m *Matcher) MatchWithNegation(relPath string, isDir bool) (matched, negate bool) {
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			// For dir-only patterns, a file matches only if it lives inside
			// a matching directory. Check the parent path against the regex.
			dir := pathDir(relPath)
			if dir == "" {
				continue
			}
			if p.regex.MatchString(dir) {
				matched = true
				negate = p.negate
			}
			continue
		}
		if p.regex.MatchString(relPath) {
			matched = true
			negate = p.negate
		}
	}
	return
}

func (m *Matcher) Match(relPath string, isDir bool) bool {
	matched, negate := m.MatchWithNegation(relPath, isDir)
	return matched && !negate
}

func (m *Matcher) merge(other *Matcher) {
	if other == nil {
		return
	}
	m.patterns = append(m.patterns, other.patterns...)
}

// Match checks fullPath against all matchers in the stack, using last-match-wins
// semantics across the whole stack (correct Git behavior: a child .gitignore
// can un-ignore what a parent ignored).
func (s Stack) Match(fullPath string, isDir bool) bool {
	ignored := false
	for _, m := range s {
		// Skip matchers whose baseDir is not an ancestor of fullPath.
		// This avoids the filepath.Rel call on most matchers for deeply
		// nested files where the stack has multiple levels.
		bd := m.baseDir
		if len(fullPath) <= len(bd) {
			if fullPath != bd {
				continue
			}
		} else if fullPath[len(bd)] != os.PathSeparator || !strings.HasPrefix(fullPath, bd) {
			continue
		}
		rel, err := filepath.Rel(bd, fullPath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			continue
		}
		if matched, negate := m.MatchWithNegation(rel, isDir); matched {
			ignored = !negate
		}
	}
	return ignored
}

// Push returns a new Stack with m appended.
func (s Stack) Push(m *Matcher) Stack {
	if m == nil || len(m.patterns) == 0 {
		return s
	}
	result := make(Stack, len(s)+1)
	copy(result, s)
	result[len(s)] = m
	return result
}

// MergeFromDir loads .gitignore and .git/info/exclude from a directory and merges
// them into a single Matcher.
func MergeFromDir(dir string) (*Matcher, error) {
	m, err := LoadFromDir(dir)
	if err != nil {
		return nil, err
	}
	ex, err := LoadGitExclude(dir)
	if err != nil {
		return m, nil
	}
	if m == nil {
		return ex, nil
	}
	m.merge(ex)
	return m, nil
}

// MergeFromDirWithEntries is like MergeFromDir but uses an existing directory
// listing to check for .git/ existence, avoiding a stat syscall on every
// directory that doesn't have a .git/ (i.e. every non-root subdirectory).
// Handles submodule .git files (gitdir: pointers) in addition to regular
// .git/ directories.
func MergeFromDirWithEntries(dir string, entries []os.DirEntry) (*Matcher, error) {
	m, err := LoadFromDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			gitDir := resolveGitDir(dir)
			if gitDir == "" {
				break
			}
			excludePath := filepath.Join(gitDir, "info", "exclude")
			if _, err := os.Stat(excludePath); err == nil {
				ex, err := LoadFromFile(dir, excludePath)
				if err == nil && ex != nil {
					if m == nil {
						return ex, nil
					}
					m.merge(ex)
				}
			}
			break
		}
	}
	return m, nil
}

// resolveGitDir returns the absolute path to the git metadata directory for the
// given working directory. Handles:
//   - regular .git/ directory          → dir/.git
//   - submodule .git file (gitdir:)    → resolves the pointer
func resolveGitDir(dir string) string {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return dotGit
	}

	// Submodule: .git is a file containing "gitdir: <path>"
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitPath := strings.TrimSpace(line[len(prefix):])
	if !filepath.IsAbs(gitPath) {
		gitPath = filepath.Join(dir, gitPath)
	}
	gitPath = filepath.Clean(gitPath)
	if resolved, err := filepath.EvalSymlinks(gitPath); err == nil {
		return resolved
	}
	return gitPath
}

// pathDir returns the parent directory of a forward-slash path, or "" if the
// path has no parent (is a bare filename at the root of the matcher).
func pathDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return ""
	}
	return p[:idx]
}
