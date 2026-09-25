package analyze

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dundee/gdu/v5/pkg/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoGitignoreParallel(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n*.log\n"), 0o644))
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "error.log"), "error\n")
	writeTestFile(t, filepath.Join(root, "node_modules", "pkg.js"), "module.exports = {}\n")
	writeTestFile(t, filepath.Join(root, "src", "app.go"), "package src\n")
	writeTestFile(t, filepath.Join(root, "src", "debug.log"), "debug\n")

	a := CreateAnalyzer()
	a.SetAutoGitignore(true)
	item := a.AnalyzeDir(root, func(name, path string) bool { return false }, nil)
	a.wait.Wait()
	dir := item.(*Dir)

	names := giItemNames(dir)
	assert.Contains(t, names, "main.go")
	assert.Contains(t, names, "src")
	assert.NotContains(t, names, "node_modules", "node_modules should be gitignored")
	assert.NotContains(t, names, "error.log", "*.log should be gitignored")

	srcDir := giFindItem(dir, "src").(*Dir)
	srcNames := giItemNames(srcDir)
	assert.Contains(t, srcNames, "app.go")
	assert.NotContains(t, srcNames, "debug.log", "*.log should be gitignored in src/")
}

func TestAutoGitignoreNested(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub", "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", ".gitignore"), []byte("*.bak\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "deep", ".gitignore"), []byte("secret/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub", "deep", "secret"), 0o755))

	writeTestFile(t, filepath.Join(root, "a.tmp"), "tmp")
	writeTestFile(t, filepath.Join(root, "keep.txt"), "ok")
	writeTestFile(t, filepath.Join(root, "sub", "b.bak"), "bak")
	writeTestFile(t, filepath.Join(root, "sub", "keep.go"), "ok")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "c.tmp"), "tmp")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "keep.go"), "ok")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "secret", "hidden.txt"), "secret")

	a := CreateAnalyzer()
	a.SetAutoGitignore(true)
	item := a.AnalyzeDir(root, func(name, path string) bool { return false }, nil)
	a.wait.Wait()
	dir := item.(*Dir)

	allFiles := giCollectAll(t, dir, "")
	assert.NotContains(t, allFiles, "a.tmp", "root *.tmp ignored")
	assert.Contains(t, allFiles, "keep.txt")
	assert.NotContains(t, allFiles, "b.bak", "sub *.bak ignored")
	assert.Contains(t, allFiles, "sub/keep.go")
	assert.NotContains(t, allFiles, "c.tmp", "nested *.tmp still applies from root")
	assert.NotContains(t, allFiles, "hidden.txt", "secret dir ignored by deep .gitignore")
}

func TestAutoGitignoreDisabledByDefault(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644))
	writeTestFile(t, filepath.Join(root, "error.log"), "error")

	a := CreateAnalyzer()
	item := a.AnalyzeDir(root, func(name, path string) bool { return false }, nil)
	a.wait.Wait()
	dir := item.(*Dir)

	names := giItemNames(dir)
	assert.Contains(t, names, "error.log", "error.log should be visible without --auto-gitignore")
}

func TestAutoGitignoreChildUnignore(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "logs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "logs", ".gitignore"), []byte("!keep.log\n"), 0o644))

	writeTestFile(t, filepath.Join(root, "a.log"), "a")
	writeTestFile(t, filepath.Join(root, "logs", "error.log"), "err")
	writeTestFile(t, filepath.Join(root, "logs", "keep.log"), "keep")

	a := CreateAnalyzer()
	a.SetAutoGitignore(true)
	item := a.AnalyzeDir(root, func(name, path string) bool { return false }, nil)
	a.wait.Wait()
	dir := item.(*Dir)

	allFiles := giCollectAll(t, dir, "")
	assert.NotContains(t, allFiles, "a.log")
	assert.NotContains(t, allFiles, "error.log")
	assert.Contains(t, allFiles, "logs/keep.log", "child .gitignore should un-ignore keep.log")
}

func TestAutoGitignoreSequential(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644))
	writeTestFile(t, filepath.Join(root, "main.go"), "ok")
	writeTestFile(t, filepath.Join(root, "error.log"), "err")

	a := CreateSeqAnalyzer()
	a.SetAutoGitignore(true)
	item := a.AnalyzeDir(root, func(name, path string) bool { return false }, nil)
	dir := item.(*Dir)

	names := giItemNames(dir)
	assert.NotContains(t, names, "error.log", "error.log should be ignored by sequential analyzer")
	assert.Contains(t, names, "main.go")
}

// helpers (prefixed gi to avoid collision with existing test helpers)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func giItemNames(dir *Dir) []string {
	var names []string
	for _, item := range dir.Files {
		names = append(names, item.GetName())
	}
	return names
}

func giFindItem(dir *Dir, name string) fs.Item {
	for _, item := range dir.Files {
		if item.GetName() == name {
			return item
		}
	}
	return nil
}

func giCollectAll(t *testing.T, dir *Dir, prefix string) []string {
	t.Helper()
	var names []string
	for _, item := range dir.Files {
		full := item.GetName()
		if prefix != "" {
			full = prefix + "/" + full
		}
		names = append(names, full)
		if item.IsDir() {
			names = append(names, giCollectAll(t, item.(*Dir), full)...)
		}
	}
	return names
}
