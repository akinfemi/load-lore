// Package mermaid builds Mermaid flowcharts. Callers add nodes, edges and
// classes; the builder handles ids, escaping and layout of the text, so no
// caller assembles Mermaid syntax by hand.
package mermaid

import (
	"fmt"
	"regexp"
	"strings"
)

// Direction of the flowchart.
type Direction string

// Directions.
const (
	LeftRight Direction = "LR"
	TopDown   Direction = "TD"
)

// Shape of a node.
type Shape int

// Node shapes.
const (
	Box      Shape = iota // [label]
	Rounded               // (label)
	Stadium               // ([label])
	Cylinder              // [(label)]
	Hexagon               // {{label}}
)

// EdgeStyle of an edge.
type EdgeStyle int

// Edge styles.
const (
	Solid  EdgeStyle = iota // -->
	Dotted                  // -.->
)

type node struct {
	id    string
	lines []string
	shape Shape
}

type edge struct {
	from, to string
	lines    []string
	style    EdgeStyle
}

type classDef struct{ name, style string }

// Flowchart is a Mermaid flowchart under construction.
type Flowchart struct {
	dir       Direction
	nodes     []node
	index     map[string]int
	edges     []edge
	classDefs []classDef
	classes   map[string][]string // class name -> node ids, in insertion order
	classOrd  []string
}

// New returns an empty flowchart.
func New(dir Direction) *Flowchart {
	return &Flowchart{dir: dir, index: map[string]int{}, classes: map[string][]string{}}
}

var reserved = map[string]bool{
	"end": true, "graph": true, "flowchart": true, "subgraph": true, "style": true,
	"class": true, "classdef": true, "click": true, "linkstyle": true, "default": true,
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_]`)

// ID turns a name into a safe Mermaid node id. Distinct design names stay
// distinct: they never contain '_', and '-' is the only character replaced.
func ID(name string) string {
	id := unsafeID.ReplaceAllString(name, "_")
	if id == "" || reserved[strings.ToLower(id)] || (id[0] >= '0' && id[0] <= '9') {
		id = "n_" + id
	}
	return id
}

// Node adds a node, or replaces the label and shape of an existing one.
// Each label line is plain text; the builder escapes it.
func (f *Flowchart) Node(id string, shape Shape, lines ...string) {
	if i, ok := f.index[id]; ok {
		f.nodes[i] = node{id: id, lines: lines, shape: shape}
		return
	}
	f.index[id] = len(f.nodes)
	f.nodes = append(f.nodes, node{id: id, lines: lines, shape: shape})
}

// Edge adds an edge with an optional multi-line label.
func (f *Flowchart) Edge(from, to string, style EdgeStyle, lines ...string) {
	f.edges = append(f.edges, edge{from: from, to: to, lines: lines, style: style})
}

// ClassDef defines a class, such as "fill:#d4edda,stroke:#28a745".
func (f *Flowchart) ClassDef(name, style string) {
	f.classDefs = append(f.classDefs, classDef{name, style})
}

// Class assigns a class to a node.
func (f *Flowchart) Class(id, class string) {
	if _, ok := f.classes[class]; !ok {
		f.classOrd = append(f.classOrd, class)
	}
	f.classes[class] = append(f.classes[class], id)
}

// String renders the flowchart as Mermaid.
func (f *Flowchart) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "flowchart %s\n", f.dir)
	for _, n := range f.nodes {
		open, closing := shapeDelims(n.shape)
		fmt.Fprintf(&b, "    %s%s\"%s\"%s\n", n.id, open, quoted(n.lines), closing)
	}
	for _, e := range f.edges {
		arrow := "-->"
		if e.style == Dotted {
			arrow = "-.->"
		}
		if len(e.lines) == 0 {
			fmt.Fprintf(&b, "    %s %s %s\n", e.from, arrow, e.to)
			continue
		}
		fmt.Fprintf(&b, "    %s %s|\"%s\"| %s\n", e.from, arrow, quoted(e.lines), e.to)
	}
	for _, c := range f.classDefs {
		fmt.Fprintf(&b, "    classDef %s %s\n", c.name, c.style)
	}
	for _, c := range f.classOrd {
		fmt.Fprintf(&b, "    class %s %s\n", strings.Join(f.classes[c], ","), c)
	}
	return b.String()
}

// Plain renders a reduced dialect for terminal renderers: square nodes,
// unquoted labels, no classes. Characters with meaning in Mermaid syntax are
// dropped from labels.
func (f *Flowchart) Plain() string {
	var b strings.Builder
	fmt.Fprintf(&b, "graph %s\n", f.dir)
	for _, n := range f.nodes {
		fmt.Fprintf(&b, "%s[%s]\n", n.id, plain(n.lines, "<br/>"))
	}
	for _, e := range f.edges {
		arrow := "-->"
		if e.style == Dotted {
			arrow = "-.->"
		}
		if len(e.lines) == 0 {
			fmt.Fprintf(&b, "%s %s %s\n", e.from, arrow, e.to)
			continue
		}
		// Non-breaking spaces keep terminal renderers from drawing the
		// edge line through the gaps between words.
		label := strings.ReplaceAll(plain(e.lines, " · "), " ", "\u00a0")
		fmt.Fprintf(&b, "%s %s|%s| %s\n", e.from, arrow, label, e.to)
	}
	return b.String()
}

func shapeDelims(s Shape) (string, string) {
	switch s {
	case Rounded:
		return "(", ")"
	case Stadium:
		return "([", "])"
	case Cylinder:
		return "[(", ")]"
	case Hexagon:
		return "{{", "}}"
	}
	return "[", "]"
}

var quoteEscaper = strings.NewReplacer(`"`, "#quot;", "<", "#lt;", ">", "#gt;")

func quoted(lines []string) string {
	esc := make([]string, len(lines))
	for i, l := range lines {
		esc[i] = quoteEscaper.Replace(l)
	}
	return strings.Join(esc, "<br/>")
}

var plainDrop = strings.NewReplacer(`"`, "", "[", "", "]", "", "(", "", ")", "", "{", "", "}", "", "|", "/", "<", "", ">", "", ";", ",", "#", "")

func plain(lines []string, sep string) string {
	esc := make([]string, len(lines))
	for i, l := range lines {
		esc[i] = plainDrop.Replace(l)
	}
	return strings.Join(esc, sep)
}
