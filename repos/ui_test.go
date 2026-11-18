package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

func makeRepo(t *testing.T, dir, readme string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-q", "-m", "initial commit")
	run(t, dir, "git", "remote", "add", "origin", "git@github.com:"+filepath.Base(filepath.Dir(dir))+"/"+filepath.Base(dir)+".git")
}

// drain runs cmd and every command it produces, feeding messages back into m.
// Ticks (blink, flash timeout) are skipped so this terminates.
func drain(m *model, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case clearFlashMsg, nil:
		default:
			// Cursor blink messages re-arm themselves forever; skip them.
			if strings.Contains(strings.ToLower(fmt.Sprintf("%T", msg)), "blink") {
				continue
			}
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

func newTestModel(t *testing.T) (*model, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "github.com/alice/webapp"), "# Webapp\n\nA small web application.\n")
	makeRepo(t, filepath.Join(root, "github.com/bob/cli-tool"), "# CLI tool\n")
	makeRepo(t, filepath.Join(root, "gitlab.com/team/infra"), "# Infra\n")
	os.WriteFile(filepath.Join(root, "github.com/alice/webapp/new.txt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "notes"), 0o755)

	recordHistory(filepath.Join(root, "gitlab.com/team/infra"))

	rels, err := listRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(root, 2, orderByHistory(root, rels), lipgloss.NewRenderer(io.Discard), true, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	drain(m, m.Init())
	return m, root
}

func screen(m *model) string { return ansi.Strip(m.View()) }

func typeText(m *model, s string) {
	for _, r := range s {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		drain(m, cmd)
	}
}

func key(m *model, k tea.KeyType) {
	_, cmd := m.Update(tea.KeyMsg{Type: k})
	drain(m, cmd)
}

func TestPickerView(t *testing.T) {
	m, _ := newTestModel(t)
	out := screen(m)
	t.Log("\n" + out)

	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[1], "gitlab.com/team/infra") {
		t.Errorf("recently used repo should be listed first, got %q", lines[1])
	}
	for _, want := range []string{
		"4/4",                     // counter
		"github.com/alice/webapp", // listed
		"notes",                   // plain folder still listed
		"Info", "Tree",            // preview tabs
		"main", "no upstream", "✓ clean", // status of selected (infra)
		"git@github.com:team/infra.git", // remote
		"initial commit",                // recent commits
		"README", "Infra",
		"↵ cd", "^o edit", "^d delete",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}

	typeText(m, "webapp")
	out = screen(m)
	t.Log("\n" + out)
	if !strings.Contains(out, "1/4") {
		t.Errorf("filter should leave 1 match")
	}
	for _, want := range []string{"● 1 changed", "Changes", "?? new.txt", "A small web application."} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q after filtering", want)
		}
	}

	key(m, tea.KeyTab)
	out = screen(m)
	if !strings.Contains(out, "README.md") || !strings.Contains(out, "webapp/") {
		t.Errorf("tree tab should list files:\n%s", out)
	}
}

func TestPickerDelete(t *testing.T) {
	m, root := newTestModel(t)
	typeText(m, "cli-tool")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	drain(m, cmd)
	if out := screen(m); !strings.Contains(out, "Delete") || !strings.Contains(out, "[y/N]") {
		t.Fatalf("expected confirmation prompt:\n%s", out)
	}

	// Anything but y cancels.
	typeText(m, "n")
	if _, err := os.Stat(filepath.Join(root, "github.com/bob/cli-tool")); err != nil {
		t.Fatal("repo deleted without confirmation")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	typeText(m, "y")
	if _, err := os.Stat(filepath.Join(root, "github.com/bob")); !os.IsNotExist(err) {
		t.Errorf("repo and empty owner dir should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "github.com/alice/webapp")); err != nil {
		t.Errorf("sibling repo must survive: %v", err)
	}
	if out := screen(m); !strings.Contains(out, "● deleted") || !strings.Contains(out, "0/3") {
		t.Errorf("expected deleted flash and item removed:\n%s", out)
	}
}

func TestPickerEnter(t *testing.T) {
	m, root := newTestModel(t)
	typeText(m, "alweb")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if want := filepath.Join(root, "github.com/alice/webapp"); m.chosen != want {
		t.Errorf("chosen = %q, want %q", m.chosen, want)
	}
	if cmd == nil {
		t.Error("enter should quit")
	}
}

func TestPickerHostFilter(t *testing.T) {
	m, _ := newTestModel(t)
	ctrlT := func() {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
		drain(m, cmd)
	}
	names := func() []string {
		var out []string
		for _, mt := range m.matches {
			out = append(out, m.items[mt.item].rel)
		}
		return out
	}

	// github.com has the most repos, so it comes first.
	ctrlT()
	if m.host != "github.com" || len(m.matches) != 2 || !strings.Contains(screen(m), "@github.com") {
		t.Fatalf("after 1st ctrl-t: host=%q matches=%v", m.host, names())
	}
	ctrlT()
	if m.host != "gitlab.com" || len(m.matches) != 1 {
		t.Fatalf("after 2nd ctrl-t: host=%q matches=%v", m.host, names())
	}
	ctrlT()
	if m.host != "" || len(m.matches) != 4 {
		t.Fatalf("after 3rd ctrl-t should show all: host=%q matches=%v", m.host, names())
	}

	typeText(m, "@gl")
	if got := names(); len(got) != 1 || got[0] != filepath.FromSlash("gitlab.com/team/infra") {
		t.Errorf("@gl = %v", got)
	}

	m.input.SetValue("")
	typeText(m, "@gh cli-t")
	if got := names(); len(got) != 1 || got[0] != filepath.FromSlash("github.com/bob/cli-tool") {
		t.Errorf("@gh cli-t = %v", got)
	}

	// ctrl-t and @ combine: gitlab.com + @gh matches nothing.
	ctrlT()
	ctrlT()
	if len(m.matches) != 0 {
		t.Errorf("gitlab.com + @gh should match nothing, got %v", names())
	}
}

func TestSplitQuery(t *testing.T) {
	hosts, q := splitQuery("@gh  web @ app")
	if len(hosts) != 1 || hosts[0] != "gh" || q != "web app" {
		t.Errorf("splitQuery = %q, %q", hosts, q)
	}
}
