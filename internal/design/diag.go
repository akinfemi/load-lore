package design

import (
	"fmt"
	"sort"
	"strconv"
)

// Severity of a diagnostic.
type Severity string

// Severities. Errors make a design invalid; warnings don't.
const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

// Diagnostic codes. They are stable: the AI skill and CI may match on them.
const (
	CodeSyntax         = "yaml-syntax"
	CodeType           = "type"
	CodeRequired       = "required"
	CodeUnknownField   = "unknown-field"
	CodeOutcomeField   = "outcome-field"
	CodePlanned        = "planned"
	CodeDuplicateKey   = "duplicate-key"
	CodeInvalidValue   = "invalid-value"
	CodeRange          = "range"
	CodeUnknownService = "unknown-service"
	CodeConflict       = "conflict"
	CodeNotApplicable  = "not-applicable"
	CodeOpTarget       = "op-target"
	CodeCycle          = "cycle"
	CodeKeyspace       = "keyspace"
	CodeTiming         = "timing"
	CodeExpectSyntax   = "expect-syntax"
	CodeExpectMetric   = "expect-metric"
	CodeExpectValue    = "expect-value"
	CodeAssumptionPath = "assumption-path"
	CodeUnusedPool     = "unused-pool"
	CodeScale          = "scale"
	CodeNoLB           = "no-lb"
)

// Diagnostic is one problem found in a design file.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
}

func (d Diagnostic) String() string {
	loc := ""
	if d.Line > 0 {
		loc = fmt.Sprintf("%d:%d: ", d.Line, d.Column)
	}
	path := ""
	if d.Path != "" {
		path = d.Path + ": "
	}
	return fmt.Sprintf("%s%s%s: %s", loc, path, d.Severity, d.Message)
}

// Diagnostics is a list of diagnostics in file order.
type Diagnostics []Diagnostic

// Errors returns only the errors.
func (ds Diagnostics) Errors() Diagnostics { return ds.filter(SevError) }

// Warnings returns only the warnings.
func (ds Diagnostics) Warnings() Diagnostics { return ds.filter(SevWarning) }

// HasErrors reports whether any diagnostic is an error.
func (ds Diagnostics) HasErrors() bool { return len(ds.Errors()) > 0 }

func (ds Diagnostics) filter(s Severity) Diagnostics {
	out := Diagnostics{}
	for _, d := range ds {
		if d.Severity == s {
			out = append(out, d)
		}
	}
	return out
}

func (ds Diagnostics) sorted() Diagnostics {
	out := append(Diagnostics(nil), ds...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Line == 0) != (b.Line == 0) {
			return a.Line != 0
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return out
}

// path helpers. Paths look like services.api.behavior.calls[1].target.

func pathKey(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func pathIndex(base string, i int) string {
	return base + "[" + strconv.Itoa(i) + "]"
}
