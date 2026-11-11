package design

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseExpect(t *testing.T) {
	tests := []struct {
		in        string
		subject   string
		op        CmpOp
		num       float64
		unit      string
		qualifier QualifierKind
		cond      string
		after     time.Duration
	}{
		{in: "p99_ms < 300 until load_rps >= 3000", subject: "p99_ms", op: OpLT, num: 300, qualifier: Until, cond: "load_rps >= 3000"},
		{in: "error_rate < 1% throughout", subject: "error_rate", op: OpLT, num: 1, unit: "%", qualifier: Throughout},
		{in: "cache.hit_rate > 80% after 1m", subject: "cache.hit_rate", op: OpGT, num: 80, unit: "%", qualifier: After, after: time.Minute},
		{in: "db.connections <= 96", subject: "db.connections", op: OpLE, num: 96, qualifier: Throughout},
		{in: "p95_ms>=1.5s", subject: "p95_ms", op: OpGE, num: 1.5, unit: "s", qualifier: Throughout},
		{in: "rps >= 3,000 until load_rps >= 4_000", subject: "rps", op: OpGE, num: 3000, qualifier: Until, cond: "load_rps >= 4000"},
		{in: "my-api.p99_ms < 250ms after 1m30s", subject: "my-api.p99_ms", op: OpLT, num: 250, unit: "ms", qualifier: After, after: 90 * time.Second},
		{in: "  p50_ms < .5  ", subject: "p50_ms", op: OpLT, num: 0.5, qualifier: Throughout},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			p, err := ParseExpect(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if p.Subject.String() != tt.subject || p.Op != tt.op || p.Value.Number != tt.num || p.Value.Unit != tt.unit {
				t.Errorf("got %s %s %v%s", p.Subject, p.Op, p.Value.Number, p.Value.Unit)
			}
			if p.Qualifier.Kind != tt.qualifier {
				t.Errorf("qualifier %s, want %s", p.Qualifier.Kind, tt.qualifier)
			}
			if tt.cond != "" {
				c := p.Qualifier.Cond
				got := c.Metric.String() + " " + string(c.Op) + " " + fmtNum(c.Value.Number)
				if got != tt.cond {
					t.Errorf("condition %q, want %q", got, tt.cond)
				}
			}
			if p.Qualifier.After.D() != tt.after {
				t.Errorf("after %v, want %v", p.Qualifier.After.D(), tt.after)
			}
		})
	}
}

func TestParseExpectErrors(t *testing.T) {
	tests := []struct {
		in     string
		offset int
		msg    string
	}{
		{"", 0, "expected a metric such as p99_ms or db.connections, found the end of the line"},
		{"p99_ms", 6, "expected <, <=, > or >=, found the end of the line"},
		{"p99_ms 300", 7, `expected <, <=, > or >=, found "300"`},
		{"p99_ms < fast", 9, `expected a number, found "fast"`},
		{"p99_ms == 300", 7, `"==" isn't supported: use <, <=, > or >=`},
		{"p99_ms != 300", 7, `"!=" isn't supported: use <, <=, > or >=`},
		{"p99_ms < 300 sometimes", 13, `unexpected "sometimes": expected until, after or throughout`},
		{"p99_ms < 300 until", 18, "expected a condition such as load_rps >= 3000, found the end of the line"},
		{"p99_ms < 300 until load_rps", 27, "expected <, <=, > or >=, found the end of the line"},
		{"p99_ms < 300 after", 18, `expected a time after "after", such as 1m`},
		{"p99_ms < 300 after 5", 19, `expected a time after "after", such as 1m: duration "5" needs a unit, such as 5s or 5m`},
		{"p99_ms < 300 throughout please", 24, `unexpected "please" at the end of the line`},
		{"p99_ms < 300kb", 12, `unknown unit "kb": use %, ms or s, or no unit`},
		{"a.b.c < 3", 0, `invalid metric "a.b.c": write <metric> or <service>.<metric>`},
		{"p99_ms < 3 @", 11, `unexpected "@"`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseExpect(tt.in)
			var se *ExpectSyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("got %v, want a syntax error", err)
			}
			if se.Offset != tt.offset || se.Msg != tt.msg {
				t.Errorf("got %d %q\nwant %d %q", se.Offset, se.Msg, tt.offset, tt.msg)
			}
		})
	}
}

func TestCmpOpHolds(t *testing.T) {
	for _, tt := range []struct {
		op   CmpOp
		a, b float64
		want bool
	}{
		{OpLT, 1, 2, true}, {OpLT, 2, 2, false}, {OpLE, 2, 2, true},
		{OpGT, 3, 2, true}, {OpGT, 2, 2, false}, {OpGE, 2, 2, true},
	} {
		if got := tt.op.Holds(tt.a, tt.b); got != tt.want {
			t.Errorf("%v %s %v = %v", tt.a, tt.op, tt.b, got)
		}
	}
}

func TestParseRunMetricForms(t *testing.T) {
	p, err := ParseExpect("break_load_rps within 25% of 1,000")
	if err != nil || p.Op != OpWithin || p.Value.Number != 1000 || p.Tolerance.Text != "25%" {
		t.Errorf("within: %+v, %v", p, err)
	}
	if p, err := ParseExpect("errors_load_rps none"); err != nil || p.Op != OpNone {
		t.Errorf("none: %+v, %v", p, err)
	}
	for line, want := range map[string]string{
		"break_load_rps within 25% 1000":      `expected "of"`,
		"break_load_rps within 25% of 1000 x": `unexpected "x"`,
		"break_load_rps none please":          `unexpected "please"`,
	} {
		if _, err := ParseExpect(line); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", line, err, want)
		}
	}
}
