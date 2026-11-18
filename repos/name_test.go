package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepoPath(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/user/proj":                 "github.com/user/proj",
		"https://github.com/user/proj/":                "github.com/user/proj",
		"https://github.com/user/proj.git":             "github.com/user/proj",
		"git@github.com:user/proj.git":                 "github.com/user/proj",
		"github.com:user/proj":                         "github.com/user/proj",
		"ssh://git@gitlab.com:2222/group/sub/proj.git": "gitlab.com/group/sub/proj",
		"https://user:tok@host.io/a/b":                 "host.io/a/b",
	} {
		got, err := repoPath(in)
		if err != nil || got != want {
			t.Errorf("repoPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"/local/proj", "./proj", "https://github.com", "git@host:../x", "https://h/a/../../x"} {
		if got, err := repoPath(in); err == nil {
			t.Errorf("repoPath(%q) = %q; want error", in, got)
		}
	}
}

func TestListRepos(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{
		"github.com/user/proj/.git",
		"github.com/user/proj/sub/inner/.git", // nested inside a repo: ignored
		"gitlab.com/group/sub/proj/.git",
		"legacy/.git",
		"plain",
		".hidden/x/.git",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := listRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github.com/user/proj", "gitlab.com/group/sub/proj", "legacy", "plain"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listRepos = %v, want %v", got, want)
	}
}

func TestParseStatus(t *testing.T) {
	st := parseStatus(`# branch.oid abc
# branch.head feature/x
# branch.upstream origin/feature/x
# branch.ab +2 -1
1 .M N... 100644 100644 100644 aaa bbb src/main go.go
2 R. N... 100644 100644 100644 aaa bbb R100 new.go	old.go
? untracked file.txt`)
	if st.Branch != "feature/x" || !st.Upstream || st.Ahead != 2 || st.Behind != 1 {
		t.Errorf("branch info = %+v", st)
	}
	want := []string{" M src/main go.go", "R  old.go → new.go", "?? untracked file.txt"}
	if !reflect.DeepEqual(st.Changes, want) {
		t.Errorf("changes = %q, want %q", st.Changes, want)
	}
}

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:user/proj.git":       "https://github.com/user/proj",
		"https://github.com/user/proj.git":   "https://github.com/user/proj",
		"https://user:tok@gitlab.com/g/p":    "https://gitlab.com/g/p",
		"ssh://git@host:2222/group/proj.git": "https://host/group/proj",
		"":                                   "",
	} {
		if got := webURL(in); got != want {
			t.Errorf("webURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	recordHistory("/a")
	recordHistory("/b")
	recordHistory("/a")
	if got := loadHistory(); !reflect.DeepEqual(got, []string{"/a", "/b"}) {
		t.Errorf("history = %v", got)
	}
	forgetHistory("/a")
	if got := loadHistory(); !reflect.DeepEqual(got, []string{"/b"}) {
		t.Errorf("history after forget = %v", got)
	}
}

func TestProgressRegex(t *testing.T) {
	for line, want := range map[string]string{
		"remote: Counting objects:  45% (45/100)":                 "Counting objects",
		"Receiving objects:  12% (12/100), 1.20 MiB | 3.00 MiB/s": "Receiving objects",
		"Resolving deltas: 100% (10/10), done.":                   "Resolving deltas",
		"Updating files:  50% (5/10)":                             "Updating files",
	} {
		if m := progressRe.FindStringSubmatch(line); m == nil || m[1] != want {
			t.Errorf("progressRe(%q) = %v, want %q", line, m, want)
		}
	}
	for _, line := range []string{"fatal: repository not found", "warning: you appear to have cloned an empty repository."} {
		if phaseRe.MatchString(line) {
			t.Errorf("phaseRe should not match %q", line)
		}
	}
}

func TestShortWhen(t *testing.T) {
	for in, want := range map[string]string{
		"5 months ago": "5mo", "1 year, 2 months ago": "1y", "3 seconds ago": "3s", "2 weeks ago": "2w", "weird": "weird",
	} {
		if got := shortWhen(in); got != want {
			t.Errorf("shortWhen(%q) = %q, want %q", in, got, want)
		}
	}
}
