# How gtu reads .gitignore files

## TL;DR

`--auto-gitignore` (on by default) makes gtu honor `.gitignore` during the
directory walk. Every directory entered is checked for a `.gitignore`. Patterns
stack with last-match-wins semantics — identical to how Git itself behaves.

## Discovery model

The walker (`processDir` in `pkg/analyze/parallel.go` and `sequential.go`)
loads `.gitignore` **per-directory, as it descends**:

```
enters
 │
 ├─ a/b/                          ← no .gitignore, nothing loaded, stack=[]
 │   │
 │   ├─ c/                        ← c/.gitignore found, pushed onto stack
 │   │   │                          stack=[c/.gitignore]
 │   │   │
 │   │   ├─ node_modules/         ← matches "node_modules/" → PRUNED
 │   │   │   └─ (never entered)
 │   │   │
 │   │   ├─ src/                  ← not ignored → entered
 │   │   │   │                      stack=[c/.gitignore] (inherited, no new .gitignore)
 │   │   │   ├─ app.go            ← not ignored → scanned
 │   │   │   └─ debug.log         ← matches "*.log" → PRUNED
 │   │   │
 │   │   └─ build/                ← matches "build/" → PRUNED
 │   │       └─ (never entered)
 │   │
 │   └─ notes.txt                 ← not ignored → scanned
 │
 stack shown at each level is what the walker holds when deciding
 whether to enter / prune each child entry.
```

There is no pre-scan or separate pass. The `.gitignore` is loaded inside the
same `processDir` call that lists the directory contents (`parallel.go:99`).
This means:

- Only directories actually visited pay the cost.
- A pruned subtree's `.gitignore` is never read (the directory isn't entered).
- The scan root's `.gitignore` is loaded regardless of whether the root is a
  git repo.

## What gets loaded

For each directory, `gitignore.MergeFromDirWithEntries` checks:

1. **`.gitignore`** in the directory — loaded if present.
2. **`.git/info/exclude`** — loaded when a `.git` exists (see "Submodule
   support" below). This is the repo-local exclude file, not a committed
   `.gitignore`.

Both are merged into a single `Matcher` before being pushed onto the stack.

## Submodule support

In a normal repo, `.git` is a directory:

```
myproject/.git/info/exclude   ← exists, loaded
```

In a **submodule**, `.git` is a **file** containing a `gitdir:` pointer:

```
_submodules/midscene/.git     ← file: "gitdir: ../../.git/modules/_submodules/midscene"
```

`resolveGitDir()` (`pkg/gitignore/gitignore.go`) handles both:

| Input `.git`      | Resolution                                   |
|-------------------|----------------------------------------------|
| Directory         | Use `.git/` directly                          |
| File `gitdir:`    | Read pointer, resolve relative to worktree, follow symlinks |

This means submodule `.git/info/exclude` patterns work correctly even though
the `.git` entry is a file, not a directory.

Without this, a scan of `/parent` would correctly apply
`_submodules/foo/.gitignore` (loaded from the file), but silently ignore
`_submodules/foo/.git/info/exclude`.

## Pattern stacking

Patterns are evaluated **last-match-wins** across the entire stack, matching
Git semantics:

```
root/.gitignore:     *.log
sub/.gitignore:      !important.log
```

Result: `important.log` inside `sub/` is **not** ignored (child un-ignores
what parent ignored).

The `Stack` is an immutable slice — `Push` returns a new copy (`gitignore.go:145`).
In the parallel analyzer this is critical: each goroutine gets its own stack
copy, so concurrent walks don't share backing arrays.

## Where it's wired

| Component            | File                              | What it does                        |
|----------------------|-----------------------------------|-------------------------------------|
| `BaseAnalyzer`       | `pkg/analyze/analyzer.go`         | `autoGitignore` flag, `SetAutoGitignore()` |
| Parallel walker      | `pkg/analyze/parallel.go:99`      | Loads per-dir, passes stack to children |
| Sequential walker    | `pkg/analyze/sequential.go:76`    | Same pattern                         |
| TopDir walker        | `pkg/analyze/parallel_top_dir.go` | Native gitignore in `AnalyzeDir` + `processSubDir`, no analyzer swap needed |
| CLI flag             | `cmd/gdu/main.go:86`              | `--auto-gitignore` (default `true`)  |
| App wiring           | `cmd/gdu/app/app.go`              | `SetAutoGitignore` on analyzer for all UI modes |
| gitignore package    | `pkg/gitignore/gitignore.go`      | `Matcher`, `Stack`, `resolveGitDir` |
| Pattern compiler     | `pkg/gitignore/pattern.go`        | Glob → regex translation            |

## Performance note

All three analyzers (`ParallelAnalyzer`, `SequentialAnalyzer`,
`TopDirAnalyzer`) implement gitignore natively — no analyzer swap is needed
when `--auto-gitignore` is active.

`TopDirAnalyzer` (the default for non-interactive stdout output) originally
lacked gitignore support and had to be swapped for the heavier
`ParallelAnalyzer`. This is no longer the case: the stack is checked during
the lightweight size-accumulation walk in both `AnalyzeDir` (top-level files)
and `processSubDir` (recursive subtrees), with the same immutably-copied
`gitignore.Stack` pattern as `ParallelAnalyzer`.

## Disabling

```
gtu --auto-gitignore=false /path
```

When disabled, the `autoGitignore` field stays `false`, `MergeFromDirWithEntries`
is never called, and the stack remains empty — zero overhead.
