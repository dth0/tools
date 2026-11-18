package main

import (
	"strings"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
)

// renderMarkdown renders a README with glamour for a pane width columns wide.
// The theme and colour profile come from the picker's own terminal renderer:
// glamour's auto-detection inspects stdout, which the shell function captures.
func renderMarkdown(src string, width int, dark bool, profile termenv.Profile) (string, error) {
	style := glamourstyles.LightStyleConfig
	if dark {
		style = glamourstyles.DarkStyleConfig
	}
	// The preview box already has padding; drop glamour's document margin and
	// the blank lines it puts around the document.
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithColorProfile(profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", err
	}
	out, err := r.Render(src)
	if err != nil {
		return "", err
	}
	return strings.Trim(out, "\n"), nil
}
