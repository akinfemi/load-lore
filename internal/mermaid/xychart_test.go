package mermaid

import "testing"

func TestXYChart(t *testing.T) {
	c := &XYChart{Title: `Say "hi"`, XLabel: "seconds", XMax: 20, YLabel: "ms", YMax: 100}
	c.Bar("#eee", []float64{0, 100, 0})
	c.Line("#123", []float64{1.23456, 2, 1.0 / 3})
	want := `---
config:
  themeVariables:
    xyChart:
      plotColorPalette: "#eee, #123"
---
xychart-beta
    title "Say 'hi'"
    x-axis "seconds" 0 --> 20
    y-axis "ms" 0 --> 100
    bar [0, 100, 0]
    line [1.235, 2, 0.333]
`
	if got := c.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
