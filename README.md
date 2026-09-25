# go TokenUsage() — gtu

Fast disk usage and **token count** analyzer written in Go.

Gtu estimates how many tokens each file would consume in an LLM context window
(using tiktoken's `o200k_base` BPE), while also tracking traditional disk usage
and apparent size. Three metrics are available in the TUI, switched with the
`a` key: **tokens** → **disk usage** → **apparent size**.

Tokens are counted for text files (via BPE tokenization) and images (via
dimension-based vision formula — PNG/JPEG/GIF/WebP/BMP). Binary files and
documents are skipped.

[![Go Report Card](https://goreportcard.com/badge/github.com/dundee/gdu)](https://goreportcard.com/report/github.com/dundee/gdu)

## Installation

Binary releases are on the [releases page](https://github.com/dundee/gdu/releases).
Build from source:

```
git clone https://github.com/dundee/gdu
cd gdu
make build
./dist/gtu
```

## Usage

```
  gtu [flags] [directory_to_scan...]

Flags:
      --archive-browsing               Enable browsing of zip/jar/tar archives
      --auto-gitignore                 Honor .gitignore files (including nested ones) during scanning (default true)
      --collapse-path                  Collapse single-child directory chains
      --config-file string             Read config from file (default is $HOME/.gtu.yaml)
      --ctrl-c-quits                   Quit gtu when Ctrl+C is pressed during a scan
  -D, --db string                      Store analysis in database (*.sqlite / *.badger)
      --depth int                      Show directory structure up to specified depth
      --enable-profiling               Enable pprof profiling server on :6060
  -E, --exclude-type strings           File types to exclude (e.g. --exclude-type yaml,json)
  -L, --follow-symlinks                Follow symlinks for files
  -h, --help                           help for gtu
  -i, --ignore-dirs strings            Paths to ignore (default /proc,/dev,/sys,/run)
  -I, --ignore-dirs-pattern strings    Path patterns to ignore (regex, comma-separated)
  -X, --ignore-from string             Read path patterns (regex, one per line) from file
  -G, --ignore-from-gitignore string   Read directories to ignore from .gitignore-style file
  -f, --input-file string              Import analysis from JSON file
      --interactive                    Force interactive mode even when output is not a TTY
  -l, --log-file string                Path to a logfile (default "/dev/null")
      --max-age string                 Include files with mtime no older than DURATION
  -m, --max-cores int                  Set max cores that Gtu will use
      --min-age string                 Include files with mtime at least DURATION old
      --mouse                          Use mouse
  -c, --no-color                       Do not use colorized output
      --no-confirm-quit                Do not ask for confirmation before quitting
  -x, --no-cross                       Do not cross filesystem boundaries
      --no-delete                      Do not allow deletions
  -H, --no-hidden                      Ignore hidden directories (beginning with dot)
      --no-prefix                      Show sizes as raw numbers without any prefixes
  -p, --no-progress                    Do not show progress in non-interactive mode
      --no-spawn-shell                 Do not allow spawning shell
  -u, --no-unicode                     Do not use Unicode symbols (for size bar)
      --no-view-file                   Do not allow viewing file contents
  -n, --non-interactive                Do not run in interactive mode
      --output-attrs string            Export selected JSON attributes (name,asize,dsize,items,mtime,notreg,tokens)
  -o, --output-file string             Export all info into file as JSON
  -r, --read-from-storage              Use existing database instead of re-scanning
      --reverse-sort                   Reverse sorting order in non-interactive mode
      --sequential                     Use sequential scanning (intended for rotating HDDs)
  -A, --show-annexed-size              Use apparent size of git-annex'ed files
  -a, --show-apparent-size             Show apparent size
  -d, --show-disks                     Show all mounted disks
  -k, --show-in-kib                    Show sizes in KiB (or kB with --si)
  -C, --show-item-count                Show number of items in directory
  -M, --show-mtime                     Show latest mtime of items in directory
  -B, --show-relative-size             Show relative size
      --show-symlink-target            Show symlink target (name -> target)
      --show-tokens                    Count and show token estimates (default true)
      --si                             Show sizes with decimal SI prefixes (kB, MB, GB)
      --since string                   Include files with mtime >= WHEN
  -s, --summarize                      Show only a total in non-interactive mode
  -t, --top int                        Show only top X largest files in non-interactive mode
      --trash-command string           Command used to move items to trash
  -T, --type strings                   File types to include (e.g. --type yaml,json)
      --until string                   Include files with mtime <= WHEN
  -v, --version                        Print version
      --web                            Run the web UI (browser interface)
      --web-listen string              Address for web UI (default: localhost + random port)
      --web-open                       Open web UI in browser on start (default true)
      --write-config                   Write current configuration to file

Basic list of actions in interactive mode:
  ↑ or k                              Move cursor up
  ↓ or j                              Move cursor down
  → or Enter or l                     Go to highlighted directory
  ← or h                              Go to parent directory
  a                                   Cycle: token count / disk usage / apparent size
  n                                   Sort by name
  s                                   Sort by size (respects active display metric)
  c                                   Show/hide item count
  m                                   Show/hide latest mtime
  d                                   Delete the selected file or directory
  D                                   Move the selected file or directory to trash
  e                                   Empty the selected directory
  ?                                   Show help modal
```

## Examples

```
gtu                                   # analyze current dir (shows token counts by default)
gtu --show-tokens=false               # analyze with traditional disk usage display
gtu --auto-gitignore=false /          # disable .gitignore honoring
gtu --show-tokens=false -a            # show apparent size instead of disk usage
gtu --no-delete                       # prevent write operations
gtu ~/projects/alpha ~/projects/beta  # analyze several dirs at once
gtu -d                                # show all mounted disks
gtu -i /sys,/proc /                   # ignore some paths
gtu -I '.*[abc]+'                     # ignore paths by regular pattern
gtu -X ignore_file /                  # ignore paths by regular patterns from file
gtu -G .gitignore /                   # ignore dirs by .gitignore-style patterns from file
gtu --auto-gitignore=false -G .gitignore /  # explicit .gitignore only, no auto-discovery
gtu -n /                              # only print stats, do not start interactive mode
gtu --reverse-sort -n /               # print files sorted smallest to largest
gtu -o- / | gzip -c >report.json.gz   # write all info to JSON
gtu --web /                           # analyze and browse results in a web browser
```

## Token counting

By default the interactive TUI shows **estimated token counts** for every file
and directory, using:

- **Text files** — BPE tokenization via tiktoken (`o200k_base`, GPT-4o encoding)
  with an offline embedded vocabulary. Binary files are detected and skipped.
- **Images** (PNG, JPEG, GIF, WebP, BMP) — dimension headers are read from the
  file, and token cost is estimated using the GPT-4V vision formula:
  `85 + 170 * ceil(w/512) * ceil(h/512)` after resizing to max 1024px.
- **Documents, audio, video, archives** — currently skipped (tokens = 0). Only
  text and image files are token-counted.

Press `a` to cycle between token count / disk usage / apparent size. The sort
key `s` follows the active display metric. The `--show-tokens=false` flag
disables token counting and restores the original disk-usage-first behavior.

## Automatic .gitignore support

By default gtu automatically discovers and honors `.gitignore` files during
scanning. Use `--auto-gitignore=false` to disable:

```
gtu ~/projects/myapp                   # .gitignore honored by default
gtu --auto-gitignore=false ~/projects  # disable .gitignore honoring
```

This works recursively: if a subdirectory has its own `.gitignore`, those
patterns are applied **on top of** the parent's patterns, with correct Git
last-match-wins semantics (a child can un-ignore files a parent ignored).
`.git/info/exclude` is also loaded when present.

Combined with the explicit `-G` flag:

```
gtu -G /path/to/extra.gitignore /some/dir
```

## Modes

Three modes: **interactive** (default), **non-interactive**, and **export**.

Non-interactive mode is triggered automatically when stdout is not a TTY, or
explicitly with `-n`.

Export mode (`-o`) writes analysis as JSON. The export includes token counts
when the `tokens` attribute is enabled (via `--output-attrs`).

## Configuration

Gtu reads YAML config from `$HOME/.gtu.yaml`, `$HOME/.config/gtu/gtu.yaml`,
and `/etc/gtu.yaml`.

```yaml
show-tokens: true
sorting:
    by: size
    order: desc
```

## Alternatives

- [ncdu](https://dev.yorhel.nl/ncdu) — NCurses-based tool in `C`/`Zig`
- [dua](https://github.com/Byron/dua-cli) — `Rust` with similar interface
- [dust](https://github.com/bootandy/dust) — `Rust` tree-like disk usage
- [pdu](https://github.com/KSXGitHub/parallel-disk-usage) — `Rust` parallel
- [diskus](https://github.com/sharkdp/diskus) — Very fast `Rust` analyzer