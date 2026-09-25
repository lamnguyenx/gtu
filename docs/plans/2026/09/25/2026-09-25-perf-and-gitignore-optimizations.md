# Performance pass: token counting, auto-gitignore, and submodule support

**Date:** 2026-09-25
**Base:** a557f06 (gtu: add token count mode, auto-gitignore, and rename gdu → gtu)

## What was done

### 1. `--show-tokens=false` was a no-op — token counting never skipped

**Problem:** The `ShowTokens` flag only controlled TUI display. BPE encoding
ran on every single text file regardless of the flag value.

**Fix:**
- Added `ignoreTokens bool` to `BaseAnalyzer` (`analyzer.go`)
- Added `SetIgnoreTokens(bool)` to the `Analyzer` interface (`analyze.go`)
- Guarded `tokencount.CountTokens()` calls with `if !a.ignoreTokens` in all
  three analyzers (`parallel.go`, `sequential.go`, `parallel_top_dir.go`)
- Wired via `app.go`: `SetIgnoreTokens(!a.Flags.ShowTokens)` for all UI modes

**Mock updates:** `MockedAnalyzer` in `internal/common/ui_test.go` and
`internal/testanalyze/analyze.go` needed `SetIgnoreTokens(v bool) {}` stubs.

**Lesson:** When adding a performance-sensitive flag, trace the full path from
CLI → app → analyzer. "Display only" flags are a common footgun.

---

### 2. Redundant `file.Stat()` inside `countTextTokens`

**Problem:** `text.go` accepted `info os.FileInfo` as `_ os.FileInfo` (ignored),
then called `file.Stat()` again to get the file size — an extra syscall per
text file.

**Fix:** Use the passed `info.Size()` directly. Changed signature param from
`_ os.FileInfo` to `info os.FileInfo`.

---

### 3. Binary extension skip list

**Problem:** Files like `.so`, `.woff`, `.pdf`, `.zip` were opened, read up to
10MB, binary-sniffed, then rejected — wasted I/O for known binary types.

**Fix:** Added `isBinaryExt()` switch in `counter.go` with 60+ known binary
extensions. Returns 0 before opening the file.

**Trial & Error — map vs switch:**
- First tried `map[string]bool` — caused unexpected GC pressure in the
  benchmark. The 10MB pool backing array lifetime interacted badly with GC
  pacing.
- Switched to `switch` statement (functionally identical, no map allocation).
  Benchmark variance between approaches was actually a PGO/artifact issue
  (see "Benchmarking pitfalls" below), but the switch is cleaner anyway.

**Lesson:** `switch` on a fixed set of extensions is cleaner and avoids map
allocation overhead for a hot path. Go inlines it well.

---

### 4. gofmt violations from commit a557f06

**Problem:** `parallel.go` and `sequential.go` had misindented `if file != nil`
blocks — the code was correct but gofmt flagged both files. `image.go` had
bitwise operator spacing (`b&0x3F<<2` → `b & 0x3F << 2`) flagged by Go 1.26's
gofmt.

**Fix:** `gofmt -w` on all three files.

---

### 5. Auto-gitignore: eliminate wasted stat syscalls

**Problem:** `MergeFromDir` called `os.Stat(".gitignore")` then
`os.Stat(".git/info/exclude")` on **every directory** entered — ~10K wasted
stats on a tree with only ~130 `.gitignore` files and zero `.git/` subdirs.

**Fix:**
- New `MergeFromDirWithEntries(dir, entries)` — uses the existing `os.ReadDir`
  listing to check for `.git/` existence instead of a separate stat. **Zero
  extra syscalls.**
- Replaced both `MergeFromDir` call sites in the analyzers.

**Also:** `AnalyzeDir` was loading the root `.gitignore` into the stack, then
passing the stack to `processDir` which loaded it **again** — duplicate
patterns for every match. Removed the redundant load from `AnalyzeDir`.

---

### 6. Auto-gitignore: submodule `.git` file support

**Problem:** In submodules, `.git` is a **file** (`gitdir: ../../.git/modules/...`),
not a directory. The `IsDir()` check meant submodule `.git/info/exclude` was
silently skipped.

**Fix:** Added `resolveGitDir()` in `gitignore.go`:
- Regular `.git/` dir → use directly
- Submodule `.git` file → read `gitdir:` pointer, resolve relative path,
  follow symlinks

**Test:** Verified on `/home/lamnt45/git/hanoi-testing-platform` with 8
submodules under `_submodules/`. Confirmed `.git/info/exclude` loaded for
`midscene` submodule.

**Lesson:** `.git` is not always a directory. Any code that checks for
`.git/info/exclude` must handle the `gitdir:` pointer case.

---

### 7. `--auto-gitignore` broken in non-interactive/export/web modes

**Problem:** `SetAutoGitignore` was only called from `getOptions()` → TUI mode.
Non-interactive, export, and web modes never set the flag.

**Root cause:** `createUI()` has a `switch` with separate code paths for each
UI type. Only the `default` (TUI) case called `getOptions()`.

**Fix:** Added `SetAutoGitignore` / `SetIgnoreTokens` calls in each branch of
`createUI()` — stdout, export, and web.

**Trial & Error — TopDirAnalyzer swap:**
- First fix: when `AutoGitignore` was set, swapped stdout's `TopDirAnalyzer`
  for `ParallelAnalyzer` (because TopDir didn't support gitignore).
- This defeated the purpose of TopDir (lightweight, no tree building).
- Better fix: added native gitignore support to `TopDirAnalyzer` itself —
  pass `gitignore.Stack` through `processSubDir`, check before entering dirs
  and processing files. Removed the swap entirely.

**Lesson:** Don't swap analyzers to work around a missing feature. Add the
feature to the analyzer that needs it. The stack is lightweight (immutable
slice copy per goroutine) and fits any walker pattern.

---

### 8. `--auto-gitignore` default changed to `true`

**Change:** Flag default flipped from `false` to `true` in `main.go`.

Updated README examples and help text to reflect the new default.
`--auto-gitignore=false` disables.

---

### 9. `Stack.Match` optimization

**Problem:** Every file/directory match call did `filepath.Clean` (redundant —
paths from `filepath.Join` are already clean) and `filepath.Rel` against every
matcher in the stack.

**Fix:**
- Removed redundant `filepath.Clean`
- Added prefix check: skip matchers whose `baseDir` isn't an ancestor of the
  path, avoiding `filepath.Rel` entirely for non-matching matchers

---

## Files changed

```
MOD: cmd/gdu/app/app.go              — SetAutoGitignore/SetIgnoreTokens for all UI modes
MOD: cmd/gdu/main.go                 — --auto-gitignore default true
MOD: pkg/analyze/analyzer.go         — ignoreTokens field, SetIgnoreTokens
MOD: pkg/analyze/parallel.go         — ignoreTokens guard, removed redundant AnalyzeDir gitignore load
MOD: pkg/analyze/sequential.go       — same
MOD: pkg/analyze/parallel_top_dir.go — native gitignore support in TopDirAnalyzer
MOD: pkg/analyze/cancel_test.go      — updated processSubDir call signature
MOD: pkg/gitignore/gitignore.go      — MergeFromDirWithEntries, resolveGitDir, Stack.Match opt
MOD: pkg/gitignore/gitignore_test.go — submodule + resolveGitDir tests
MOD: pkg/tokencount/counter.go       — isBinaryExt switch
MOD: pkg/tokencount/text.go          — remove redundant Stat, use info param
MOD: pkg/tokencount/image.go         — gofmt fix (bitwise op spacing)
MOD: internal/common/analyze.go      — SetIgnoreTokens on interface
MOD: internal/common/ui_test.go      — mock stub
MOD: internal/testanalyze/analyze.go — mock stub
MOD: README.md                       — updated for auto-gitignore default
NEW: docs/important/how-we-read-gitignore.md
NEW: docs/plans/2026/09/25/2026-09-25-perf-and-gitignore-optimizations.md
```

## Benchmarking pitfalls

Go's benchmark results were misleading during this session:

1. **PGO mismatch:** The repo has `default.pgo`. Changes to code cause the PGO
   profile to be stale, skewing results. `-pgo=off` didn't help — the issue was
   GC pacing changes from minor code layout shifts, not PGO itself.

2. **Sink elision:** Without a package-level `var sink int64` consumed by
   `b.N` loop, the compiler optimized away the entire benchmark body (1.3ns/op,
   0 allocs for file I/O — obviously wrong).

3. **Tight-loop unrepresentativeness:** BPE encoding benchmarked in isolation
   showed different relative speeds between approaches, but real-world scans
   (mixed I/O, parallel goroutines, OS page cache) showed negligible
   differences. The microbenchmark was measuring GC scheduler artifacts, not
   actual code performance.

**Lesson:** For I/O-bound tools, measure end-to-end wall time on real directory
trees, not tight-loop microbenchmarks. The 20-core NVMe test machine finished
42K-file scans in ~50ms regardless of optimizations — differences only matter
on slower disks / fewer cores.

## Test results

```
All 21 packages pass, 0 failures
pkg/gitignore: +2 new tests (submodule gitdir, resolveGitDir)
pkg/analyze:   existing tests pass with updated signatures
```
