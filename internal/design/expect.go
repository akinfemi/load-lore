package design

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
	"go.yaml.in/yaml/v3"
)

// Expectation is one line of the expect block: a prediction in the expect
// grammar, or a free-text note when the prediction can't be expressed.
//
//	<metric> <op> <value> [until <condition> | after <time> | throughout]
type Expectation struct {
	Text string // the prediction as written; empty for a note
	Note string // a free-text prediction, never scored

	// Prediction is the parsed form of Text. Nil for notes and for lines
	// that failed to parse.
	Prediction *Prediction
}

// Prediction is a parsed expect line.
type Prediction struct {
	Subject MetricRef
	Op      CmpOp
	Value   Quantity
	// Tolerance is the N% of "within N% of X" (OpWithin).
	Tolerance Quantity
	Qualifier Qualifier
}

// MetricRef names an end-to-end metric (Service empty) or a per-component
// metric, written service.metric.
type MetricRef struct {
	Service string
	Metric  string
	Offset  int // byte offset in the line, for error columns
}

func (m MetricRef) String() string {
	if m.Service == "" {
		return m.Metric
	}
	return m.Service + "." + m.Metric
}

// CmpOp is a comparison operator.
type CmpOp string

// Comparison operators accepted in expect lines.
const (
	OpLT CmpOp = "<"
	OpLE CmpOp = "<="
	OpGT CmpOp = ">"
	OpGE CmpOp = ">="
	// OpWithin is "<metric> within <N>% of <value>": held when the
	// measured value is within N% of the value, either side.
	OpWithin CmpOp = "within"
	// OpNone is "<metric> none": held when it never happened.
	OpNone CmpOp = "none"
)

// Holds reports whether "actual op threshold" is true.
func (o CmpOp) Holds(actual, threshold float64) bool {
	switch o {
	case OpLT:
		return actual < threshold
	case OpLE:
		return actual <= threshold
	case OpGT:
		return actual > threshold
	case OpGE:
		return actual >= threshold
	}
	return false
}

// Quantity is a number with the unit it was written with ("", "%", "ms", "s").
// Validation fills Canonical: milliseconds for latencies, a 0–1 fraction for
// ratios, the plain number otherwise.
type Quantity struct {
	Number    float64
	Unit      string
	Text      string
	Offset    int
	Canonical float64
}

// QualifierKind says over which part of the run a prediction is checked.
type QualifierKind string

// Qualifier kinds. A line with no qualifier means throughout.
const (
	Throughout QualifierKind = "throughout"
	Until      QualifierKind = "until"
	After      QualifierKind = "after"
)

// Qualifier limits the part of the run a prediction applies to.
type Qualifier struct {
	Kind QualifierKind
	// Until: the condition that ends the checked range.
	Cond *Condition
	// After: time from the start of the workload.
	After Duration
}

// Condition is "<metric> <op> <value>" inside an until qualifier.
type Condition struct {
	Metric MetricRef
	Op     CmpOp
	Value  Quantity
}

// ExpectSyntaxError is a parse error at a byte offset within the line.
type ExpectSyntaxError struct {
	Offset int
	Msg    string
}

func (e *ExpectSyntaxError) Error() string { return e.Msg }

// ParseExpect parses one prediction line. It checks syntax only; whether the
// metric exists for the design is checked by validation.
func ParseExpect(line string) (*Prediction, error) {
	p := &expectParser{toks: lexExpect(line), src: line}
	if err := p.lexErr(); err != nil {
		return nil, err
	}
	pred := &Prediction{Qualifier: Qualifier{Kind: Throughout}}
	var err error
	if pred.Subject, err = p.metric("a metric such as p99_ms or db.connections"); err != nil {
		return nil, err
	}
	// <metric> within <N>% of <value>, or <metric> none.
	switch t := p.toks[p.pos]; {
	case t.kind == tokWord && t.text == "none":
		p.next()
		pred.Op = OpNone
		return pred, p.end()
	case t.kind == tokWord && t.text == "within":
		p.next()
		pred.Op = OpWithin
		if pred.Tolerance, err = p.quantity(); err != nil {
			return nil, err
		}
		if of := p.next(); of.kind != tokWord || of.text != "of" {
			return nil, &ExpectSyntaxError{of.off, fmt.Sprintf("expected \"of\" after the tolerance, as in within 25%% of 1000, found %s", describe(of))}
		}
		if pred.Value, err = p.quantity(); err != nil {
			return nil, err
		}
		return pred, p.end()
	}
	if pred.Op, err = p.op(); err != nil {
		return nil, err
	}
	if pred.Value, err = p.quantity(); err != nil {
		return nil, err
	}
	if p.done() {
		return pred, nil
	}
	t := p.next()
	switch t.text {
	case "throughout":
	case "until":
		c := &Condition{}
		if c.Metric, err = p.metric("a condition such as load_rps >= 3000"); err != nil {
			return nil, err
		}
		if c.Op, err = p.op(); err != nil {
			return nil, err
		}
		if c.Value, err = p.quantity(); err != nil {
			return nil, err
		}
		pred.Qualifier = Qualifier{Kind: Until, Cond: c}
	case "after":
		d := p.next()
		if d.kind == tokEOF {
			return nil, &ExpectSyntaxError{d.off, "expected a time after \"after\", such as 1m"}
		}
		dur, derr := ParseDuration(d.text)
		if derr != nil {
			return nil, &ExpectSyntaxError{d.off, fmt.Sprintf("expected a time after \"after\", such as 1m: %v", derr)}
		}
		pred.Qualifier = Qualifier{Kind: After, After: dur}
	default:
		return nil, &ExpectSyntaxError{t.off, fmt.Sprintf("unexpected %q: expected until, after or throughout", t.text)}
	}
	if !p.done() {
		t := p.next()
		return nil, &ExpectSyntaxError{t.off, fmt.Sprintf("unexpected %q at the end of the line", t.text)}
	}
	return pred, nil
}

type tokKind int

const (
	tokEOF tokKind = iota
	tokWord
	tokNumber
	tokOp
	tokBad
)

type token struct {
	kind tokKind
	text string
	off  int
}

func lexExpect(s string) []token {
	var toks []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '<' || c == '>':
			j := i + 1
			if j < len(s) && s[j] == '=' {
				j++
			}
			toks = append(toks, token{tokOp, s[i:j], i})
			i = j
		case c == '=' || c == '!':
			j := i + 1
			for j < len(s) && (s[j] == '=' || s[j] == '<' || s[j] == '>') {
				j++
			}
			toks = append(toks, token{tokBad, s[i:j], i})
			i = j
		case isDigit(c) || (c == '.' && i+1 < len(s) && isDigit(s[i+1])):
			j := i
			for j < len(s) && (isDigit(s[j]) || s[j] == '.' || s[j] == ',' || s[j] == '_') {
				j++
			}
			for j < len(s) && (isLetter(s[j]) || s[j] == '%' || isDigit(s[j]) || s[j] == '.') {
				j++
			}
			toks = append(toks, token{tokNumber, s[i:j], i})
			i = j
		case isLetter(c):
			j := i
			for j < len(s) && (isLetter(s[j]) || isDigit(s[j]) || s[j] == '_' || s[j] == '-' || s[j] == '.') {
				j++
			}
			toks = append(toks, token{tokWord, s[i:j], i})
			i = j
		default:
			j := i + 1
			for j < len(s) && s[j] != ' ' {
				j++
			}
			toks = append(toks, token{tokBad, s[i:j], i})
			i = j
		}
	}
	return append(toks, token{tokEOF, "", len(s)})
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

type expectParser struct {
	toks []token
	pos  int
	src  string
}

func (p *expectParser) lexErr() error {
	for _, t := range p.toks {
		if t.kind != tokBad {
			continue
		}
		switch t.text {
		case "=", "==":
			return &ExpectSyntaxError{t.off, fmt.Sprintf("%q isn't supported: use <, <=, > or >=", t.text)}
		case "!=":
			return &ExpectSyntaxError{t.off, "\"!=\" isn't supported: use <, <=, > or >="}
		}
		return &ExpectSyntaxError{t.off, fmt.Sprintf("unexpected %q", t.text)}
	}
	return nil
}

func (p *expectParser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *expectParser) done() bool { return p.toks[p.pos].kind == tokEOF }

// end is nil at the end of the line, else an error at what follows.
func (p *expectParser) end() error {
	if p.done() {
		return nil
	}
	t := p.next()
	return &ExpectSyntaxError{t.off, fmt.Sprintf("unexpected %q at the end of the line", t.text)}
}

func (p *expectParser) metric(want string) (MetricRef, error) {
	t := p.next()
	if t.kind != tokWord {
		return MetricRef{}, &ExpectSyntaxError{t.off, fmt.Sprintf("expected %s, found %s", want, describe(t))}
	}
	m := MetricRef{Metric: t.text, Offset: t.off}
	if svc, metric, ok := strings.Cut(t.text, "."); ok {
		if svc == "" || metric == "" || strings.Contains(metric, ".") {
			return MetricRef{}, &ExpectSyntaxError{t.off, fmt.Sprintf("invalid metric %q: write <metric> or <service>.<metric>", t.text)}
		}
		m.Service, m.Metric = svc, metric
	}
	return m, nil
}

func (p *expectParser) op() (CmpOp, error) {
	t := p.next()
	if t.kind != tokOp {
		return "", &ExpectSyntaxError{t.off, fmt.Sprintf("expected <, <=, > or >=, found %s", describe(t))}
	}
	return CmpOp(t.text), nil
}

func (p *expectParser) quantity() (Quantity, error) {
	t := p.next()
	if t.kind != tokNumber {
		return Quantity{}, &ExpectSyntaxError{t.off, fmt.Sprintf("expected a number, found %s", describe(t))}
	}
	i := 0
	for i < len(t.text) && (isDigit(t.text[i]) || t.text[i] == '.' || t.text[i] == ',' || t.text[i] == '_') {
		i++
	}
	numText := strings.NewReplacer(",", "", "_", "").Replace(t.text[:i])
	unit := t.text[i:]
	n, err := strconv.ParseFloat(numText, 64)
	if err != nil {
		return Quantity{}, &ExpectSyntaxError{t.off, fmt.Sprintf("invalid number %q", t.text[:i])}
	}
	switch unit {
	case "", "%", "ms", "s":
	default:
		return Quantity{}, &ExpectSyntaxError{t.off + i, fmt.Sprintf("unknown unit %q: use %%, ms or s, or no unit", unit)}
	}
	return Quantity{Number: n, Unit: unit, Text: t.text, Offset: t.off}, nil
}

func describe(t token) string {
	if t.kind == tokEOF {
		return "the end of the line"
	}
	return strconv.Quote(t.text)
}

// UnmarshalYAML accepts a prediction string or a {note: "..."} mapping.
func (e *Expectation) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		e.Text = n.Value
		pred, err := ParseExpect(n.Value)
		if err != nil {
			return err
		}
		e.Prediction = pred
		return nil
	case yaml.MappingNode:
		if len(n.Content) != 2 || n.Content[0].Value != "note" || n.Content[1].Kind != yaml.ScalarNode {
			return fmt.Errorf(`expected a prediction such as "p99_ms < 300 until load_rps >= 3000", or {note: "..."}`)
		}
		e.Note = n.Content[1].Value
		return nil
	}
	return fmt.Errorf(`expected a prediction such as "p99_ms < 300 until load_rps >= 3000", or {note: "..."}`)
}

// MarshalJSON writes a prediction as its text and a note as {"note": ...}.
func (e Expectation) MarshalJSON() ([]byte, error) {
	if e.Note != "" {
		return json.Marshal(map[string]string{"note": e.Note})
	}
	return json.Marshal(e.Text)
}

// JSONSchema describes Expectation in the generated schema.
func (Expectation) JSONSchema() *jsonschema.Schema {
	note := jsonschema.NewProperties()
	note.Set("note", &jsonschema.Schema{Type: "string", Description: "A prediction the grammar can't express. Shown in the report, not scored."})
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:        "string",
				Description: "<metric> <op> <value> [until <condition> | after <time> | throughout]. No qualifier means throughout.",
				Examples:    []any{"p99_ms < 300 until load_rps >= 3000", "error_rate < 1% throughout", "cache.hit_rate > 80% after 1m"},
			},
			{
				Type:                 "object",
				Properties:           note,
				Required:             []string{"note"},
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
	}
}
