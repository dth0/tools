package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// editorCommand builds the command for $VISUAL / $EDITOR (default vi),
// opening dir, with the terminal wired to tty.
func editorCommand(dir string, tty *os.File) *exec.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	fields := strings.Fields(editor)
	cmd := exec.Command(fields[0], append(fields[1:], ".")...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	return cmd
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // reap the process; we don't care how it exits
	return nil
}

// copyToClipboard tries the usual clipboard tools and falls back to the
// OSC 52 escape sequence, which most modern terminals (and tmux) support.
func copyToClipboard(text string, tty *os.File) error {
	var candidates [][]string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		candidates = append(candidates, []string{"wl-copy"})
	}
	candidates = append(candidates,
		[]string{"pbcopy"},
		[]string{"xclip", "-selection", "clipboard"},
		[]string{"xsel", "--clipboard", "--input"},
	)
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" || runtime.GOOS == "darwin" {
		for _, c := range candidates {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if cmd.Run() == nil {
				return nil
			}
		}
	}
	if tty == nil {
		return errors.New("no clipboard available")
	}
	_, err := fmt.Fprintf(tty, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}
