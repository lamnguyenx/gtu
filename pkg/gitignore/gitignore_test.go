package gitignore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilePattern_AllTypes(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// Simple name — matches at any depth
		{"plain name at root", "foo", "foo", false, true},
		{"plain name in subdir", "foo", "a/b/foo", false, true},
		{"plain name no match", "foo", "bar", false, false},

		// Star glob
		{"star matches", "*.log", "error.log", false, true},
		{"star no cross slash", "*.log", "a/error.log", false, true},
		{"star partial", "*.log", "error.txt", false, false},

		// Directory-only — tested through Matcher, not raw regex
		{"dir-only matches dir", "build/", "build", true, true},
		{"dir-only no match file", "build/", "build", false, false},
		{"dir-only matches nested", "build/", "project/build", true, true},

		// Leading slash — anchored
		{"anchored root match", "/foo", "foo", false, true},
		{"anchored no nested", "/foo", "a/foo", false, false},
		{"anchored dir contents", "/build", "build/output.o", false, true},

		// Internal slash — anchored
		{"internal slash anchored", "src/test", "src/test", true, true},
		{"internal slash no nested", "src/test", "a/src/test", true, false},

		// Double star
		{"double star prefix", "**/node_modules", "node_modules", true, true},
		{"double star prefix deep", "**/node_modules", "a/b/node_modules", true, true},
		{"double star suffix", "dist/**", "dist/app.js", false, true},
		{"double star middle", "a/**/b", "a/b", false, true},
		{"double star middle deep", "a/**/b", "a/x/y/b", false, true},

		// Question mark
		{"question mark", "file?.txt", "file1.txt", false, true},
		{"question mark no match", "file?.txt", "file12.txt", false, false},

		// Character class
		{"char class match", "*.[ch]", "main.c", false, true},
		{"char class match h", "*.[ch]", "main.h", false, true},
		{"char class no match", "*.[ch]", "main.o", false, false},

		// Negation
		{"negate un-ignores", "!important.log", "important.log", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := compilePattern(tt.pattern)
			require.NoError(t, err)
			m := &Matcher{baseDir: "/", patterns: []Pattern{*p}}
			got := m.Match(tt.path, tt.isDir)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDirOnlyPattern(t *testing.T) {
	p, err := compilePattern("build/")
	require.NoError(t, err)
	assert.True(t, p.dirOnly)
	assert.True(t, p.regex.MatchString("build"))
	// dirOnly check is in Matcher.Match, not regex itself
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	giPath := filepath.Join(dir, ".gitignore")
	content := "# comment\n\nnode_modules/\n*.log\n!important.log\n/dist\n"
	require.NoError(t, os.WriteFile(giPath, []byte(content), 0o644))

	m, err := LoadFromFile(dir, giPath)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.patterns, 4) // comment and blank excluded

	// node_modules/ → dir
	assert.True(t, m.Match("node_modules", true))
	assert.False(t, m.Match("node_modules", false))

	// *.log → file
	assert.True(t, m.Match("error.log", false))

	// !important.log → un-ignored
	_, negate := m.MatchWithNegation("important.log", false)
	assert.True(t, negate) // last matching pattern is the negation

	// /dist → anchored to root
	assert.True(t, m.Match("dist", true))
	assert.False(t, m.Match("sub/dist", true))
}

func TestStackMatch(t *testing.T) {
	// Parent matcher: *.log ignored
	parent := &Matcher{
		baseDir: "/project",
		patterns: []Pattern{
			mustPattern("*.log"),
			mustPattern("!keep.log"),
		},
	}

	// Child matcher: *.tmp ignored
	child := &Matcher{
		baseDir: "/project/sub",
		patterns: []Pattern{
			mustPattern("*.tmp"),
		},
	}

	stack := Stack{parent, child}

	// *.log in parent dir → ignored
	assert.True(t, stack.Match("/project/error.log", false))

	// keep.log → un-ignored by parent
	assert.False(t, stack.Match("/project/keep.log", false))

	// *.tmp in child dir → ignored
	assert.True(t, stack.Match("/project/sub/debug.tmp", false))

	// *.log in child dir → ignored by parent pattern
	assert.True(t, stack.Match("/project/sub/error.log", false))

	// random file → not ignored
	assert.False(t, stack.Match("/project/src/main.go", false))
}

func TestStackChildUnignores(t *testing.T) {
	// Parent ignores everything in logs/
	parent := &Matcher{
		baseDir: "/project",
		patterns: []Pattern{
			mustPattern("logs/"),
		},
	}
	// Child un-ignores important.log
	child := &Matcher{
		baseDir: "/project/logs",
		patterns: []Pattern{
			mustPattern("!important.log"),
		},
	}

	stack := Stack{parent, child}

	// logs/ → ignored by parent
	assert.True(t, stack.Match("/project/logs", true))
	// logs/error.log → ignored by parent pattern logs/ matching logs/error.log
	assert.True(t, stack.Match("/project/logs/error.log", false))
	// logs/important.log → parent ignores, child un-ignores
	assert.False(t, stack.Match("/project/logs/important.log", false))
}

func TestStackPush(t *testing.T) {
	m1 := &Matcher{baseDir: "/", patterns: []Pattern{mustPattern("*.log")}}
	s := Stack{}
	s2 := s.Push(m1)
	assert.Len(t, s2, 1)
	assert.Len(t, s, 0) // original unchanged

	// Push nil or empty matcher doesn't grow
	s3 := s2.Push(nil)
	assert.Len(t, s3, 1)

	empty := &Matcher{baseDir: "/"}
	s4 := s2.Push(empty)
	assert.Len(t, s4, 1)
}

func TestMergeFromDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("*.tmp\n"), 0o644))

	m, err := MergeFromDir(dir)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.True(t, m.Match("error.log", false))
	assert.True(t, m.Match("debug.tmp", false))
}

func TestLoadFromDir_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadFromDir(dir)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

func TestEmptyStack(t *testing.T) {
	s := Stack{}
	assert.False(t, s.Match("/whatever", false))
}

// mustPattern compiles a pattern, panicking on error (test helper).
func mustPattern(line string) Pattern {
	p, err := compilePattern(line)
	if err != nil {
		panic(err)
	}
	return *p
}
