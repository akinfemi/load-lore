package diagram

import (
	"fmt"
	"strings"

	madiagram "github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/render"

	"github.com/akinfemi/load-lore/internal/mermaid"
)

// ASCII renders the flowchart for a terminal with box-drawing characters.
// It uses mermaid-ascii on the builder's plain dialect, so the terminal
// drawing shows the same graph as the Mermaid output.
func ASCII(f *mermaid.Flowchart) (string, error) {
	cfg, err := madiagram.NewConfig(false, "LR", "cli")
	if err != nil {
		return "", err
	}
	out, err := render.RenderDiagram(f.Plain(), cfg)
	if err != nil {
		return "", fmt.Errorf("rendering ASCII diagram: %w", err)
	}
	// Trim trailing spaces so output is stable for golden files and diffs.
	// The plain dialect uses non-breaking spaces in edge labels; turn them
	// back into spaces so copied output is plain text.
	out = strings.ReplaceAll(out, "\u00a0", " ")
	lines := strings.Split(strings.TrimRight(out, "\n "), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n", nil
}
