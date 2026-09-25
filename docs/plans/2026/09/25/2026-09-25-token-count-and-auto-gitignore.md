# Token Count Mode + Auto-Gitignore + gdu → gtu Rename

**Date:** 2026-09-25
**Branch:** master (f2a2cfb)

## What was done

### 1. Token counting engine (`pkg/tokencount/`)

New package that estimates token counts for files during the directory walk, using tiktoken-go with `o200k_base` encoding (GPT-4o) via offline embedded BPE vocabularies.

- **Text files** — read first 10MB, detect binary via null-byte sniff, encode with tiktoken
- **Images** (PNG/JPEG/GIF/BMP/WebP) — read dimension headers from first few bytes, apply vision formula `85 + 170 * ceil(w/512) * ceil(h/512)` (GPT-4V tile-based)
- **Other files** — binary content detection → tokens=0; empty files → tokens=0

Dependencies: `github.com/pkoukk/tiktoken-go`, `github.com/pkoukk/tiktoken-go-loader` (offline BPE vocab)

**Trials & Errors:**
- First attempt used heuristic estimation ported from `token-simple-count` (regex-based char-ratio), but user requested real tokenizer (`tiktoken-go`)
- tiktoken-go's offline loader (`tiktoken_loader.NewOfflineLoader()`) uses embedded vocab files — `SetBpeLoader` must be called **before** the first `GetEncoding` call or it silently falls through to default (which tries HTTP download)
- Image dimension parsing for WebP had a named-return-value type bug (`(int, int, ok bool)` where `ok` was redeclared as `bool`) — fixed  with renamed signature

### 2. Data model changes

Extended every layer of the size/usage aggregation to carry a third metric — Tokens.

| File | Change |
|---|---|
| `pkg/analyze/file.go:22` | Added `Tokens int64` to `File` struct |
| `pkg/analyze/file.go:189` | Added `tokens` to `dirTotals` |
| `pkg/analyze/file.go:231-233` | `aggregateDirEntries` sums `entry.GetTokens()` |
| `pkg/analyze/file.go:208-216` | `resolveDirStats` returns 4 values |
| `pkg/analyze/file.go:489-511` | `updateStats` stores tokens from resolved totals |
| `pkg/analyze/file.go:526-543` | `subtractStats` subtracts tokens from ancestors |
| `pkg/analyze/file.go:265-287` | `snapshotDir` copies `Tokens` field |
| `pkg/analyze/file.go:338-348` | `snapshotFile` calls `source.GetTokens()` |
| `pkg/fs/file.go:40` | Added `GetTokens() int64` to `Item` interface |
| `pkg/fs/file.go:45` | `GetItemStats` returns 4-tuple |
| `pkg/fs/file.go:20` | Added `SortByTokens` enum |
| `pkg/fs/file.go` | Added `ByTokens` sort type |

**Trials & Errors:**
- Had two choices: extend `GetItemStats` return to 4-tuple (breaking change, 6 implementations) vs add separate `GetTokens()` + separate aggregation path. Chose 4-tuple for consistency — it's what `aggregateDirEntries` already iterates, and matching the pattern avoided duplicating the entire aggregation loop.
- `SqliteItem` needed its own `tokens` field and `GetTokens()` method even though SQLite storage doesn't yet persist token counts (returns 0 for now)
- `SimpleDir` (top-dir non-interactive analyzer) needed `GetTokens() int64 { return 0 }` added
- `StoredDir.subtractStats` also needed tokens subtracted to keep dir totals consistent

### 3. Analyzer integration

Both the parallel (default) and sequential analyzers call `tokencount.CountTokens()` for every regular file during the scan.

- `pkg/analyze/parallel.go:189` — `regularFile.Tokens = tokencount.CountTokens(entryPath, info)`
- `pkg/analyze/sequential.go` — same pattern
- Token counting happens after file creation but during the scan, not in a separate pass — performance cost of reading file contents during the walk

**Trials & Errors:**
- Had to be careful to only call `CountTokens` for `*File` type assertions (not for ZipDir/TarDir which embed different structs)
- The count happens inside the goroutine for each file, so I/O is concurrent — acceptable for SSD scans

### 4. TUI display: 3-state `'a'` cycle

The `'a'` key now cycles through three display modes:
1. **Tokens** (default) — shows estimated token count per file/dir
2. **Disk Usage** — actual blocks on disk (the previous default)
3. **Apparent Size** — logical byte size

| File | Change |
|---|---|
| `internal/common/ui.go:23` | Added `ShowTokens bool` |
| `tui/format.go:27` | Added `tokens int64` to `rowMaxima` |
| `tui/format.go:32-53` | `getUsagePart` gains `ShowTokens` branch |
| `tui/format.go:135-147` | `formatColumns` gains `ShowTokens` branch |
| `tui/format.go:296-310` | Added `formatTokens()` helper with k/M/G suffixes |
| `tui/show.go:93` | Added `totalTokens` to `showDir` locals |
| `tui/show.go:155,164,187` | Token maxima accumulation + footer with `formatTokens(totalTokens)` |
| `tui/keys.go:522-537` | 3-state toggle logic |
| `tui/sort.go:61-68` | Size sort adapts to current display mode |
| `tui/show.go:30` | Help text updated |
| `tui/tui.go:151` | `ShowTokens` default removed from constructor (set via option) |

**Trials & Errors:**
- Initially set `ShowTokens: true` in `CreateUI()` constructor — broke 100+ TUI tests that expected old size display. Fixed: removed default from constructor, always set via option in `app.go`'s `getOptions()`
- The `formatTokens()` function was initially called as `ui.formatTokens` (method call) but defined as standalone function — compile error caught early
- `formatCount()` existed for item counts but had different semantics (G/M suffixes without unit) — created separate `formatTokens()` for consistency with token display

### 5. Binary rename: `gdu` → `gtu`

Binary name, user-facing strings, and config paths updated. Go module path (`github.com/dundee/gdu/v5`) left intact to avoid breaking imports.

| Changed | Old | New |
|---|---|---|
| Makefile binary output | `gdu` | `gtu` |
| Makefile release names | `gdu_linux_amd64` etc | `gtu_linux_amd64` etc |
| CLI `Use:` | `gdu [directory_to_scan...]` | `gtu [directory_to_scan...]` |
| CLI short desc | "disk usage analyzer" | "disk usage and token count analyzer" |
| Config defaults | `~/.gdu.yaml`, `~/.config/gdu/gdu.yaml`, `/etc/gdu.yaml` | `~/.gtu.yaml`, `~/.config/gtu/gtu.yaml`, `/etc/gtu.yaml` |
| TUI header | `gdu ~ Use arrow keys...` | `gtu ~ Use arrow keys...` |
| TUI quit modal | "quit gdu" | "quit gtu" |
| TUI help title | `gdu help` | `gtu help` |
| Export progname | `gdu` | `gtu` |
| Docker tag | `ghcr.io/dundee/gdu` | `ghcr.io/dundee/gtu` |
| Shell trash argv[0] | `gdu` | `gtu` |

**Trials & Errors:**
- User clarified "simply change the displayed name" — NOT the Go module path or directory names. Made the changes scoped to user-facing strings only. But Makefile's `PACKAGE` derives from `NAME`, so `NAME=gtu` broke `go build` because module path became `github.com/dundee/gtu/v5` (doesn't exist). Added `MODULE := gdu` variable to decouple.

### 6. Auto-gitignore (`pkg/gitignore/`)

New package that discovers `.gitignore` files per-directory during the walk, stacks them with correct Git semantics, and honors `negation (`!`) patterns.

**Features:**
- Per-directory `.gitignore` auto-discovery — checks every directory the walker enters
- Also loads `.git/info/exclude` when present
- Pattern stacking: most specific (deepest) patterns win
- Negation support (`!` patterns) — a child `.gitignore` can un-ignore what a parent ignored
- Full gitignore glob syntax: `*`, `**`, `?`, `[...]`, leading `/`, trailing `/`, internal `/`

**Files:**
- `pkg/gitignore/gitignore.go` — `Matcher`, `Stack`, `LoadFromFile`, `LoadFromDir`, `MergeFromDir`
- `pkg/gitignore/pattern.go` — `compilePattern()`, `buildRegex()` with glob→regex translation
- `pkg/gitignore/gitignore_test.go` — 17 unit tests for pattern matching, stacking, negation

**Integration:**
- `pkg/analyze/analyzer.go` — added `autoGitignore bool` + `SetAutoGitignore()`
- `pkg/analyze/parallel.go` — `processDir` accepts `gitignore.Stack`, loads local .gitignore, passes to children
- `pkg/analyze/sequential.go` — same pattern
- `cmd/gdu/main.go` — added `--auto-gitignore` flag
- `cmd/gdu/app/app.go` — wired via `Analyzer.SetAutoGitignore(true)`
- `internal/common/analyze.go` — added `SetAutoGitignore(bool)` to `Analyzer` interface

**Trials & Errors:**
- `dirOnly` patterns (trailing `/`) were tricky: the regex matches the directory name AND files inside it, but shouldn't match a file with the same name. Fix: for dirOnly patterns applied to files, check the parent path against the regex instead of the file's own path (i.e., a file inside an ignored dir is ignored, but a file named like the pattern is not).
- `walkDir` function name collision in test file — existing `walkDir` in `top.go` had different signature. Renamed test helpers with `gi` prefix.
- Stack `Push` returning new slice (immutable pattern) — each goroutine gets its own copy. Important for parallel analyzer: concurrent goroutines must not share the underlying slice array; Go's `append` with `copy` approach ensures no shared backing array.
- Add `SetAutoGitignore` to the `Analyzer` interface — broke `MockedAnalyzer` in two packages (`internal/common/ui_test.go` and `internal/testanalyze/analyze.go`). Added no-op implementations.

### 7. Integration tests

Added `pkg/analyze/gitignore_test.go` with 5 tests:
- `TestAutoGitignoreParallel` — root `.gitignore` applied during parallel scan
- `TestAutoGitignoreNested` — parent + child + grandchild `.gitignore` stacking
- `TestAutoGitignoreDisabledByDefault` — verifies off-by-default behavior
- `TestAutoGitignoreChildUnignore` — negation test: child un-ignores what parent ignored
- `TestAutoGitignoreSequential` — same functionality verified on sequential analyzer

## Files changed (summary)

```
 NEW: pkg/tokencount/counter.go
 NEW: pkg/tokencount/text.go
 NEW: pkg/tokencount/image.go
 NEW: pkg/gitignore/gitignore.go
 NEW: pkg/gitignore/pattern.go
 NEW: pkg/gitignore/gitignore_test.go
 NEW: pkg/analyze/gitignore_test.go
 MOD: pkg/analyze/file.go
 MOD: pkg/analyze/parallel.go
 MOD: pkg/analyze/sequential.go
 MOD: pkg/analyze/analyzer.go
 MOD: pkg/analyze/top_dir.go
 MOD: pkg/analyze/stored.go
 MOD: pkg/analyze/sqlite.go
 MOD: pkg/fs/file.go
 MOD: internal/common/ui.go
 MOD: internal/common/analyze.go
 MOD: internal/common/gitignore.go
 MOD: internal/testanalyze/analyze.go
 MOD: internal/common/ui_test.go
 MOD: tui/format.go
 MOD: tui/show.go
 MOD: tui/keys.go
 MOD: tui/sort.go
 MOD: tui/tui.go
 MOD: cmd/gdu/main.go
 MOD: cmd/gdu/app/app.go
 MOD: Makefile
 MOD: go.mod / go.sum
 DOC: docs/plans/2026/09/25/2026-09-25-token-count-and-auto-gitignore.md
```

## Test results

```
✓ github.com/dundee/gdu/v5 (all 21 packages pass)
  pkg/gitignore: 17 unit tests, 0 failures
  pkg/analyze: 5 integration tests, 0 failures
  Full suite: 0 failures across all packages
```