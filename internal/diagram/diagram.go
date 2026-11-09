// Package diagram draws a design's topology: the load generator, the
// services, and the calls between them. It is generated from the design file
// only, so it can't drift from what runs.
package diagram

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/akinfemi/loadlore/internal/design"
	"github.com/akinfemi/loadlore/internal/mermaid"
)

// LoadNode is the id of the load generator node.
const LoadNode = "load"

// Flowchart builds the topology of a valid design. Node ids are
// mermaid.ID(service name); the load generator is LoadNode.
func Flowchart(d *design.Design) *mermaid.Flowchart {
	f := mermaid.New(mermaid.LeftRight)
	order := nodeOrder(d)

	if w := d.Workload; w != nil {
		f.Node(LoadNode, mermaid.Stadium, "load", "up to "+rps(peakRate(w)))
	}
	for _, name := range order {
		addNode(f, name, d.Services[name])
	}

	if w := d.Workload; w != nil && w.Entry != "" {
		f.Edge(LoadNode, mermaid.ID(w.Entry), mermaid.Solid, mixLabel(w.Mix))
	}
	for _, name := range order {
		s := d.Services[name]
		switch {
		case s.Kind == "pgbouncer" && s.Target != "":
			f.Edge(mermaid.ID(name), mermaid.ID(s.Target), mermaid.Solid, fmt.Sprintf("pool %d", deref(s.DefaultPoolSize)))
		case s.Behavior != nil:
			for _, e := range callEdges(s) {
				lines := []string{strings.Join(e.ops, ", ")}
				if pool, ok := s.Pools[e.target]; ok {
					lines = append(lines, fmt.Sprintf("pool %d", pool))
				}
				style := mermaid.Solid
				if e.async {
					style = mermaid.Dotted
				}
				f.Edge(mermaid.ID(name), mermaid.ID(e.target), style, lines...)
			}
		}
	}
	return f
}

func addNode(f *mermaid.Flowchart, name string, s *design.Service) {
	title := name
	if s.Replicas != nil && *s.Replicas > 1 {
		title = fmt.Sprintf("%s ×%d", name, *s.Replicas)
	}
	res := resources(s.Resources)
	switch {
	case s.IsApp():
		lines := []string{title}
		if !s.IsSynthetic() {
			lines = append(lines, s.Image)
		}
		f.Node(mermaid.ID(name), mermaid.Box, append(lines, res)...)
	case s.Kind == "pgbouncer":
		f.Node(mermaid.ID(name), mermaid.Rounded, title, "pgbouncer · "+s.PoolMode, res)
	default:
		f.Node(mermaid.ID(name), mermaid.Cylinder, title, strings.TrimSpace(s.Kind+" "+s.Version), res)
	}
}

// nodeOrder lists services breadth first from the entry, then the rest by
// name, so diagrams read left to right in request order.
func nodeOrder(d *design.Design) []string {
	var order []string
	seen := map[string]bool{}
	queue := []string{}
	if d.Workload != nil {
		queue = append(queue, d.Workload.Entry)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		s, ok := d.Services[name]
		if !ok || s == nil || seen[name] {
			continue
		}
		seen[name] = true
		order = append(order, name)
		if s.Kind == "pgbouncer" {
			queue = append(queue, s.Target)
		}
		for _, e := range callEdges(s) {
			queue = append(queue, e.target)
		}
	}
	for _, name := range d.ServiceNames() {
		if !seen[name] && d.Services[name] != nil {
			order = append(order, name)
		}
	}
	return order
}

type callEdge struct {
	target string
	async  bool
	ops    []string
}

// callEdges groups a service's calls, including on_miss calls, into one edge
// per target and mode, in first-call order.
func callEdges(s *design.Service) []callEdge {
	if s.Behavior == nil {
		return nil
	}
	var edges []callEdge
	add := func(c *design.Call, onMiss bool) {
		async := c.Mode == "async"
		i := slices.IndexFunc(edges, func(e callEdge) bool { return e.target == c.Target && e.async == async })
		if i < 0 {
			edges = append(edges, callEdge{target: c.Target, async: async})
			i = len(edges) - 1
		}
		edges[i].ops = append(edges[i].ops, opLabel(c, onMiss))
	}
	var walk func(calls []*design.Call, onMiss bool)
	walk = func(calls []*design.Call, onMiss bool) {
		for _, c := range calls {
			if c == nil {
				continue
			}
			add(c, onMiss)
			walk(c.OnMiss, true)
		}
	}
	walk(s.Behavior.Calls, false)
	return edges
}

func opLabel(c *design.Call, onMiss bool) string {
	l := c.Op
	switch c.Query {
	case "range":
		l += fmt.Sprintf(" range %d", deref(c.Rows))
	case "scan":
		l += " scan"
	}
	if n := deref(c.Count); n > 1 {
		l += fmt.Sprintf(" ×%d", n)
	}
	switch c.When {
	case "write":
		l += " on writes"
	case "read":
		l += " on reads"
	}
	if onMiss {
		l += " on miss"
	}
	return l
}

func mixLabel(m *design.Mix) string {
	if m == nil || m.Read == nil || m.Write == nil {
		return "HTTP"
	}
	return fmt.Sprintf("%s read · %s write", pct(*m.Read), pct(*m.Write))
}

func peakRate(w *design.Workload) float64 {
	var peak float64
	for _, s := range w.Profile {
		switch {
		case s == nil:
		case s.Ramp != nil:
			peak = max(peak, s.Ramp.From, s.Ramp.To)
		case s.Hold != nil:
			peak = max(peak, s.Hold.RPS)
		case s.Spike != nil:
			peak = max(peak, s.Spike.To)
		}
	}
	return peak
}

func resources(r *design.Resources) string {
	if r == nil || r.CPU == nil || r.Memory == nil {
		return ""
	}
	return fmt.Sprintf("%s CPU · %s", strconv.FormatFloat(*r.CPU, 'f', -1, 64), r.Memory)
}

func pct(f float64) string {
	return strconv.FormatFloat(f*100, 'f', -1, 64) + "%"
}

// rps formats a rate with thousands separators: 5,000 rps.
func rps(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String() + " rps"
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
