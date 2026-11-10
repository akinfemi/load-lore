package mermaid

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// XYChart builds a Mermaid xychart-beta: lines and bars over a numeric x
// axis. Values are evenly spaced across the x range, one per point.
type XYChart struct {
	Title      string
	XLabel     string
	XMin, XMax float64
	YLabel     string
	YMin, YMax float64
	plots      []plot
}

type plot struct {
	bar    bool
	color  string
	values []float64
}

// Line adds a line in a color such as "#1f77b4".
func (c *XYChart) Line(color string, values []float64) {
	c.plots = append(c.plots, plot{color: color, values: values})
}

// Bar adds bars in a color.
func (c *XYChart) Bar(color string, values []float64) {
	c.plots = append(c.plots, plot{bar: true, color: color, values: values})
}

var chartText = strings.NewReplacer(`"`, "'", "\n", " ")

// String renders the chart. Plot colors are set through the chart's theme
// palette, in the order plots were added.
func (c *XYChart) String() string {
	var b strings.Builder
	colors := make([]string, len(c.plots))
	for i, p := range c.plots {
		colors[i] = p.color
	}
	fmt.Fprintf(&b, "---\nconfig:\n  themeVariables:\n    xyChart:\n      plotColorPalette: \"%s\"\n---\n", strings.Join(colors, ", "))
	b.WriteString("xychart-beta\n")
	if c.Title != "" {
		fmt.Fprintf(&b, "    title \"%s\"\n", chartText.Replace(c.Title))
	}
	fmt.Fprintf(&b, "    x-axis \"%s\" %s --> %s\n", chartText.Replace(c.XLabel), num(c.XMin), num(c.XMax))
	fmt.Fprintf(&b, "    y-axis \"%s\" %s --> %s\n", chartText.Replace(c.YLabel), num(c.YMin), num(c.YMax))
	for _, p := range c.plots {
		kind := "line"
		if p.bar {
			kind = "bar"
		}
		vals := make([]string, len(p.values))
		for i, v := range p.values {
			vals[i] = num(v)
		}
		fmt.Fprintf(&b, "    %s [%s]\n", kind, strings.Join(vals, ", "))
	}
	return b.String()
}

// num prints a value with at most three decimals and no exponent.
func num(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}
