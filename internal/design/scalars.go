package design

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"go.yaml.in/yaml/v3"
)

// Duration is a length of time written with a unit: 30s, 5m, 1h, 1m30s, 500ms.
type Duration time.Duration

var durationPattern = `^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`

// ParseDuration parses a design-file duration. A unit is required.
func ParseDuration(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return 0, fmt.Errorf("duration %q needs a unit, such as %ss or %sm", s, s, s)
	}
	d, err := time.ParseDuration(s)
	if err != nil || strings.ContainsAny(s, "µun") {
		return 0, fmt.Errorf("invalid duration %q: use a number and a unit (ms, s, m, h), such as 30s or 5m", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %q is negative", s)
	}
	return Duration(d), nil
}

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// String formats the duration compactly: 5m, 1m30s, 500ms.
func (d Duration) String() string {
	t := time.Duration(d)
	if t == 0 {
		return "0s"
	}
	if t < time.Second || t%time.Second != 0 {
		if t%time.Millisecond == 0 && t < time.Second {
			return fmt.Sprintf("%dms", t/time.Millisecond)
		}
		return t.String()
	}
	var b strings.Builder
	if h := t / time.Hour; h > 0 {
		fmt.Fprintf(&b, "%dh", h)
		t -= h * time.Hour
	}
	if m := t / time.Minute; m > 0 {
		fmt.Fprintf(&b, "%dm", m)
		t -= m * time.Minute
	}
	if s := t / time.Second; s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a duration such as 30s or 5m")
	}
	v, err := ParseDuration(n.Value)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// JSONSchema describes Duration in the generated schema.
func (Duration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Pattern:     durationPattern,
		Description: "A duration with a unit: 30s, 5m, 1h, 1m30s, 500ms.",
		Examples:    []any{"30s", "5m"},
	}
}

// Size is an amount of memory in bytes, written like Docker: 512m, 1g, 256MB.
type Size int64

var sizeRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([kKmMgG])(?:i[bB]?|[bB])?$`)

const sizePattern = `^[0-9]+(\.[0-9]+)?\s*[kKmMgG](i[bB]?|[bB])?$`

// ParseSize parses a memory size. Units are binary: 1k = 1024 bytes.
func ParseSize(s string) (Size, error) {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid size %q: use a number and a unit (k, m, g), such as 512m or 1g", s)
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	mult := map[string]float64{"k": 1 << 10, "m": 1 << 20, "g": 1 << 30}[strings.ToLower(m[2])]
	b := f * mult
	if b <= 0 || b > math.MaxInt64 {
		return 0, fmt.Errorf("size %q is out of range", s)
	}
	return Size(b), nil
}

// Bytes returns the size in bytes.
func (s Size) Bytes() int64 { return int64(s) }

// String formats the size in Docker notation, using the largest exact unit.
func (s Size) String() string {
	b := int64(s)
	switch {
	case b%(1<<30) == 0:
		return fmt.Sprintf("%dg", b>>30)
	case b%(1<<20) == 0:
		return fmt.Sprintf("%dm", b>>20)
	case b%(1<<10) == 0:
		return fmt.Sprintf("%dk", b>>10)
	}
	return fmt.Sprintf("%db", b)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Size) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a size such as 512m or 1g")
	}
	v, err := ParseSize(n.Value)
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s Size) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// JSONSchema describes Size in the generated schema.
func (Size) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Pattern:     sizePattern,
		Description: "Memory with a binary unit: 512m, 1g, 256MB.",
		Examples:    []any{"512m", "1g"},
	}
}

// TargetRef names a service, or one replica of it: api, api[0].
type TargetRef struct {
	Name  string
	Index *int
}

var targetRe = regexp.MustCompile(`^([a-z0-9]+(?:-[a-z0-9]+)*)(?:\[([0-9]+)\])?$`)

const targetPattern = `^[a-z0-9]+(-[a-z0-9]+)*(\[[0-9]+\])?$`

// ParseTargetRef parses "name" or "name[i]".
func ParseTargetRef(s string) (TargetRef, error) {
	m := targetRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return TargetRef{}, fmt.Errorf("invalid target %q: use a service name, or name[i] for one replica", s)
	}
	t := TargetRef{Name: m[1]}
	if m[2] != "" {
		i, err := strconv.Atoi(m[2])
		if err != nil {
			return TargetRef{}, fmt.Errorf("invalid replica index in %q", s)
		}
		t.Index = &i
	}
	return t, nil
}

func (t TargetRef) String() string {
	if t.Index == nil {
		return t.Name
	}
	return fmt.Sprintf("%s[%d]", t.Name, *t.Index)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *TargetRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a service name")
	}
	v, err := ParseTargetRef(n.Value)
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t TargetRef) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

// JSONSchema describes TargetRef in the generated schema.
func (TargetRef) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Pattern:     targetPattern,
		Description: "A service name, or name[i] for one replica.",
		Examples:    []any{"db", "api[0]"},
	}
}

// ConfigValue is a scalar passed through to a native config file, or an
// assumption's value: a string, number or boolean.
type ConfigValue struct {
	raw  string
	kind string // "string", "number", "bool"
}

// String returns the value as written.
func (c ConfigValue) String() string { return c.raw }

// IsZero reports whether the value was never set.
func (c ConfigValue) IsZero() bool { return c.kind == "" }

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *ConfigValue) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a single value (string, number or boolean)")
	}
	c.raw = n.Value
	switch n.ShortTag() {
	case "!!int", "!!float":
		c.kind = "number"
	case "!!bool":
		c.kind = "bool"
	default:
		c.kind = "string"
	}
	return nil
}

// MarshalJSON implements json.Marshaler, keeping the YAML type.
func (c ConfigValue) MarshalJSON() ([]byte, error) {
	switch c.kind {
	case "number":
		if f, err := strconv.ParseFloat(c.raw, 64); err == nil {
			return json.Marshal(f)
		}
	case "bool":
		var b bool
		if err := yaml.Unmarshal([]byte(c.raw), &b); err == nil {
			return json.Marshal(b)
		}
	}
	return json.Marshal(c.raw)
}

// JSONSchema describes ConfigValue in the generated schema.
func (ConfigValue) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{{Type: "string"}, {Type: "number"}, {Type: "boolean"}},
	}
}

func (Pool) shorthandField() string { return "size" }

// JSONSchema describes Pool in the generated schema: a size, or the
// object form.
func (Pool) JSONSchema() *jsonschema.Schema {
	props := jsonschema.NewProperties()
	props.Set("size", &jsonschema.Schema{Type: "integer", Minimum: json.Number("1"), Default: DefaultPool, Description: "Connections per replica."})
	props.Set("min", &jsonschema.Schema{Type: "integer", Minimum: json.Number("0"), Default: 0,
		Description: "Connections each replica opens before the load starts and keeps open: a warm pool. postgres and pgbouncer targets only. Default 0: connections open only when callers need them."})
	return &jsonschema.Schema{OneOf: []*jsonschema.Schema{
		{Type: "integer", Minimum: json.Number("1"), Description: "Connections per replica."},
		{Type: "object", Properties: props, AdditionalProperties: jsonschema.FalseSchema},
	}}
}
