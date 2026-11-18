# repos

A fuzzy picker and cloner for your local git repositories.

`repos` keeps every clone under one base directory (`~/Repos` by default),
organised by where it came from, and gives you a terminal UI to jump between
them. You can see each repo's git status at a glance and act on a repo
without leaving the picker.

```
❯ fuz                                    @github.com  2/34  ~/Repos
▌ github.com/sahilm/fuzzy         master ╭──────────────────────────────────────╮
  github.com/alice/webapp       ● main   │ Info │ Tree                          │
  github.com/bob/cli-tool   ↑2 ↓1 main   │ github.com/sahilm/fuzzy               │
                                         │  master · up to date · ✓ clean       │
                                         │ https://github.com/sahilm/fuzzy       │
                                         │                                       │
                                         │ Recent commits                        │
                                         │   89d593d 5mo  Add test coverage for… │
                                         │                                       │
                                         │ README                                │
                                         │  fuzzy                                │
                                         │ Go library that provides fuzzy …      │
                                         ╰───────────────────────────────────────╯

↵ cd · ^o edit · ^b browse · ^y copy · ^d delete · ^t host · tab info/tree · esc quit
```

## Features

- **Fuzzy search** over every repository below the base directory. Repos you
  used recently are listed first.
- **Git status in the list**: branch, `●` for uncommitted changes, `↑`/`↓`
  for commits ahead of or behind the upstream. Status loads in the
  background, so the picker opens instantly.
- **Preview pane** with two tabs:
  - **Info**: status, remote, changed files, the last five commits, and the
    README rendered as Markdown.
  - **Tree**: the file tree.
- **Actions** without leaving the picker: open in your editor, open the
  remote in the browser, copy the path, or delete the repo.
- **Host filter**: cycle through hosts with `ctrl-t`, or type `@gh`, `@gl`, …
  in the search.
- **`clone`** derives the target directory from the URL and shows a compact
  progress bar.
- Optional **Nerd Font icons**.

## Install

Requires Go and `git`.

```sh
make install                     # installs to ~/.local/bin/repos
make install BINDIR=/usr/local/bin   # or somewhere else
make uninstall                   # removes it again (same BINDIR rules)
```

The build must run with `CGO_ENABLED=0`, which the Makefile sets for you. One
dependency imports `os/user`, so with cgo enabled the build needs a C
compiler. With cgo disabled you get a static binary and no compiler is
needed.

Make sure the install directory is on your `PATH`.

## Shell setup

A program can't change its parent shell's working directory. So the `repos`
binary prints the path you picked, and a shell function of the same name
`cd`s into it. Add this to your `~/.zshrc` or `~/.bashrc`, after the line
that puts `repos` on your `PATH`:

```sh
eval "$(repos --init zsh)"    # or: repos --init bash
```

This defines:

```sh
repos() {
  local dir
  dir="$(command repos "$@")" || return
  [ -n "$dir" ] && cd "$dir"
}
```

The function shadows the binary, and `command repos` skips the function to
run the binary. Without the function, `repos` only prints the path.

## Usage

### Pick a repository

```sh
repos
```

Type to filter, then press Enter to `cd` into the selection. Esc leaves you
where you were.

| Key | Action |
|---|---|
| `enter` | `cd` into the repo |
| `ctrl-o` | Open the repo in `$VISUAL` / `$EDITOR` (default `vi`). When the editor exits, you land in the repo. |
| `ctrl-b` | Open the `origin` remote in your browser |
| `ctrl-y` | Copy the repo's path to the clipboard |
| `ctrl-d` | Delete the repo. Asks `[y/N]` first, and warns about uncommitted or unpushed work. |
| `ctrl-t` | Cycle the host filter: all → github.com → gitlab.com → … → all |
| `tab` | Switch the preview between **Info** and **Tree** |
| `↑` `↓` / `ctrl-p` `ctrl-n` / `ctrl-k` `ctrl-j` | Move the selection |
| `pgup` `pgdown` `home` `end` | Jump through the list |
| `esc` / `ctrl-c` | Quit without changing directory |

#### Searching

- Plain text is fuzzy-matched against the path relative to the base
  directory, for example `ghuserproj` → `github.com/user/proj`.
- Terms starting with `@` filter by host instead, also fuzzily:

  ```
  @gl               only gitlab.com repos
  @gh webapp        GitHub repos matching "webapp"
  ```

  `@` terms can appear anywhere in the query, and they combine with the
  `ctrl-t` filter.

The preview pane hides itself when the terminal is narrower than 80 columns.

#### What gets listed

`repos` walks the base directory up to 5 levels deep:

- Every directory containing `.git` is listed by its relative path. `repos`
  doesn't look inside repositories.
- Hidden directories are skipped.
- Top-level folders that aren't repositories and contain none are listed
  too, so a flat layout like `~/Repos/dotfiles` keeps working.

### Clone a repository

```sh
repos clone https://github.com/user/proj        # → ~/Repos/github.com/user/proj, then cd there
repos clone git@github.com:user/proj.git        # → same place
repos clone <url> work/proj                     # → ~/Repos/work/proj (explicit path)
```

The target directory is `<base>/<host>/<path>`, derived from the URL:

| URL | Directory |
|---|---|
| `https://github.com/user/proj` | `github.com/user/proj` |
| `git@github.com:user/proj.git` | `github.com/user/proj` |
| `ssh://git@gitlab.com:2222/group/sub/proj.git` | `gitlab.com/group/sub/proj` |
| `https://user:token@host.io/a/b` | `host.io/a/b` |

The user, credentials, port and `.git` suffix are dropped, and subgroups are
kept. URLs containing `..` are rejected so a clone can't land outside the
base directory. Local paths have no host, so for those you must pass the
directory explicitly.

On a terminal, `clone` shows one animated progress line and finishes with
`✓ cloned into … (1.2s)`. git's full output is printed only if the clone
fails. Credential and SSH host-key prompts still work. If the target already
exists, `clone` refuses to run. After a failed clone, the empty `host/owner`
folders are removed.

## Configuration

| Flag | Environment | Default | Description |
|---|---|---|---|
| `--base <dir>` | `REPOS_DIR` | `~/Repos` | Base directory for picking and cloning |
| `--icons` | `REPO_ICONS=1` | off | Show Nerd Font icons (needs a patched font) |
| `--depth <n>` | | `2` | Depth of the file tree in the **Tree** tab |
| `--init <shell>` | | | Print the shell function (`bash`, `zsh`) and exit |

Flags go before the subcommand: `repos --base ~/work clone <url>`.

To enable icons permanently, add `export REPO_ICONS=1` to your shell
config.

### Files

| Path | Purpose |
|---|---|
| `$XDG_STATE_HOME/repos/history` (default `~/.local/state/repos/history`) | Recently used repos, most recent first, one absolute path per line (max 500). Safe to delete. |

## Notes

- **Exit codes:** 0 when a repo was picked or cloned, 130 when the picker was
  cancelled, 1 on errors.
- **Read-only status checks:** git status runs with `GIT_OPTIONAL_LOCKS=0`,
  so the picker never takes `index.lock` in your repos. Each git call times
  out after 5 seconds.
- **Clipboard:** uses `wl-copy`, `pbcopy`, `xclip` or `xsel`, whichever is
  available. Without any of them, it falls back to the OSC 52 terminal escape
  sequence, which works over SSH and in tmux when the terminal supports it.
- **Browser:** `xdg-open` on Linux, `open` on macOS. HTTPS and SSH remotes
  are both converted to a web URL.
- **Colours** follow your terminal's light or dark background. The picker
  draws on `/dev/tty`, so it works even while the shell function captures
  stdout.

## Development

```sh
make test     # go test ./...
make build    # ./repos
```

| File | Contents |
|---|---|
| `main.go` | Flags, subcommands, shell integration |
| `ui.go` | Bubble Tea picker: model, keys, rendering |
| `clone.go` | `clone` command and progress display |
| `paths.go` | URL → path mapping, repository discovery |
| `git.go` | git status parsing, preview info, web URLs |
| `history.go` | Recently-used history |
| `actions.go` | Editor, browser, clipboard |
| `markdown.go` | README rendering with glamour |
| `icons.go` | Nerd Font icon table |
| `styles.go` | Colour palette |
| `tree.go` | File tree preview |

The tests in `ui_test.go` drive the picker model with key presses against
real git repositories in a temporary directory.
