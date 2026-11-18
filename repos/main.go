// repos is a fuzzy project picker and cloner for ~/Repos.
//
// A child process can't change its parent shell's working directory, so repos
// prints the selected path on stdout and a small shell function does the cd:
//
//	eval "$(repos --init zsh)"   # defines the `repos` shell function
//
// Usage:
//
//	repos [flags]                     pick a repository
//	repos [flags] clone <url> [path]  clone <url> into <base>/<host>/<owner>/<repo>
//	                                  (or <base>/<path>) and print its path
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const shellInit = `repos() {
  local dir
  dir="$(command repos "$@")" || return
  [ -n "$dir" ] && cd "$dir"
}
`

// exitCancelled is returned when the user dismisses the picker.
const exitCancelled = 130

func main() {
	base := flag.String("base", defaultBase(), "directory containing the repositories (env: REPOS_DIR)")
	depth := flag.Int("depth", 2, "tree depth shown in the preview pane")
	initShell := flag.String("init", "", "print shell integration for `shell` (bash, zsh) and exit")
	icons := flag.Bool("icons", envBool("REPO_ICONS"), "show Nerd Font icons; needs a patched font (env: REPO_ICONS=1)")
	flag.Usage = usage
	flag.Parse()

	if *initShell != "" {
		switch *initShell {
		case "bash", "zsh", "sh":
			fmt.Print(shellInit)
			return
		default:
			fatal(fmt.Errorf("unsupported shell %q (supported: bash, zsh)", *initShell))
		}
	}

	root, err := filepath.EvalSymlinks(expandHome(*base))
	if err != nil {
		fatal(err)
	}

	switch flag.Arg(0) {
	case "clone":
		err = clone(root, flag.Args()[1:])
	case "":
		err = pick(root, *depth, *icons)
	default:
		err = fmt.Errorf("unknown command %q", flag.Arg(0))
	}
	if err != nil {
		fatal(err)
	}
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  repos [flags]                     pick a repository")
	fmt.Fprintln(out, "  repos [flags] clone <url> [path]  clone <url> into <base>/<host>/<owner>/<repo>")
	fmt.Fprintln(out, "\nPicker keys:")
	fmt.Fprintln(out, "  enter cd · ctrl-o editor · ctrl-b browser · ctrl-y copy path · ctrl-d delete")
	fmt.Fprintln(out, "  ctrl-t cycle host filter · tab info/tree · esc quit")
	fmt.Fprintln(out, "  type @host (e.g. @gl) in the search to filter by host")
	fmt.Fprintln(out, "\nFlags:")
	flag.PrintDefaults()
}

func envBool(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func defaultBase() string {
	if v := os.Getenv("REPOS_DIR"); v != "" {
		return v
	}
	return "~/Repos"
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// tildify abbreviates the home directory as ~ for display.
func tildify(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "repos:", err)
	os.Exit(1)
}
