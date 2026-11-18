package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// clone runs `git clone <url> <root>/<host>/<path>` and prints the new path.
// Progress goes to stderr so stdout carries only the path for the shell
// function to cd into.
func clone(root string, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: repo clone <url> [path]")
	}
	url := args[0]
	var rel string
	var err error
	if len(args) == 2 {
		rel, err = cleanRel(args[1])
	} else {
		rel, err = repoPath(url)
	}
	if err != nil {
		return err
	}

	dest := filepath.Join(root, rel)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if err := runClone(url, dest); err != nil {
		removeEmptyParents(filepath.Dir(dest), root)
		return fmt.Errorf("git clone failed: %w", err)
	}
	recordHistory(dest)
	fmt.Println(dest)
	return nil
}

// runClone runs git clone. On a terminal it shows a single animated progress
// line; git's full output is only printed if the clone fails.
func runClone(url, dest string) error {
	cmd := exec.Command("git", "clone", "--progress", "--", url, dest)
	cmd.Stdin = os.Stdin
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		return cmd.Run()
	}

	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = w, w
	p := newProgress(os.Stderr)
	p.header(url)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		w.Close()
		r.Close()
		return err
	}
	w.Close()

	var log bytes.Buffer
	sc := bufio.NewScanner(r)
	sc.Split(splitLines)
	for sc.Scan() {
		line := sc.Text()
		log.WriteString(line + "\n")
		p.update(line)
	}
	r.Close()
	err = cmd.Wait()
	p.stop()

	if err != nil {
		p.finish(p.st.red.Render("✗") + " clone failed")
		os.Stderr.Write(log.Bytes())
		return err
	}
	p.finish(fmt.Sprintf("%s %s %s",
		p.st.green.Render("✓"),
		"cloned into "+p.st.bold.Render(tildify(dest)),
		p.st.faint.Render(fmt.Sprintf("(%.1fs)", time.Since(start).Seconds()))))
	return nil
}

// splitLines splits on \n or \r, since git redraws progress with \r.
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

var (
	// git's progress phases are capitalised ("Receiving objects: 45% ..."),
	// unlike its "fatal:", "warning:" and "error:" messages.
	progressRe = regexp.MustCompile(`^(?:remote:\s*)?([A-Z][a-z]+(?: [a-z]+)*):\s+(\d+)%`)
	phaseRe    = regexp.MustCompile(`^(?:remote:\s*)?([A-Z][a-z]+(?: [a-z]+)*):`)
	speedRe    = regexp.MustCompile(`\|\s*([\d.]+ \S+/s)`)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const barWidth = 24

type progress struct {
	out     *os.File
	st      styles
	mu      sync.Mutex
	phase   string
	percent int // -1 when unknown
	speed   string
	frame   int
	started bool
	done    chan struct{}
	wg      sync.WaitGroup
}

func newProgress(out *os.File) *progress {
	// ANSI palette colours follow the terminal theme, so unlike the picker we
	// never have to query the terminal's background colour. That query can't
	// be answered reliably here while git is also using the terminal.
	r := lipgloss.NewRenderer(out)
	return &progress{out: out, st: newANSIStyles(r), percent: -1, done: make(chan struct{})}
}

func (p *progress) header(url string) {
	fmt.Fprintf(p.out, "%s %s\n", p.st.accent.Render("↓"), "Cloning "+p.st.bold.Render(url))
}

// update consumes one line of git output. The spinner only starts ticking
// once git reports progress, so it never redraws over a credential or SSH
// host-key prompt that git writes to the terminal before the transfer.
func (p *progress) update(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m := progressRe.FindStringSubmatch(line); m != nil {
		p.phase = m[1]
		fmt.Sscan(m[2], &p.percent)
		if s := speedRe.FindStringSubmatch(line); s != nil {
			p.speed = s[1]
		}
	} else if m := phaseRe.FindStringSubmatch(line); m != nil && !strings.HasPrefix(line, "Cloning into") {
		p.phase, p.percent, p.speed = m[1], -1, ""
	} else {
		return
	}
	if !p.started {
		p.started = true
		p.wg.Add(1)
		go p.tick()
	}
	p.draw()
}

func (p *progress) tick() {
	defer p.wg.Done()
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.mu.Lock()
			p.frame++
			p.draw()
			p.mu.Unlock()
		}
	}
}

// draw must be called with p.mu held.
func (p *progress) draw() {
	line := p.st.accent.Render(spinnerFrames[p.frame%len(spinnerFrames)]) + " " + p.phase
	if p.percent >= 0 {
		filled := p.percent * barWidth / 100
		line += "  " + p.st.accent.Render(strings.Repeat("━", filled)) +
			p.st.faint.Render(strings.Repeat("━", barWidth-filled)) +
			fmt.Sprintf(" %3d%%", p.percent)
	}
	if p.speed != "" && strings.HasPrefix(p.phase, "Receiving") {
		line += "  " + p.st.faint.Render(p.speed)
	}
	fmt.Fprint(p.out, "\r\x1b[2K"+line)
}

func (p *progress) stop() {
	close(p.done)
	p.wg.Wait()
}

func (p *progress) finish(msg string) {
	fmt.Fprint(p.out, "\r\x1b[2K"+msg+"\n")
}
