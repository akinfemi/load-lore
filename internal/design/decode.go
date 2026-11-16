package design

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// The decoder walks the YAML node tree alongside the Go types. It reports
// every problem with a field path and position instead of stopping at the
// first, and enforces the same `jsonschema` struct tags (required, enum,
// ranges, patterns) that generate `loadl schema`, so the two can't drift.

// outcomeFields are names people reach for to set a result directly. They
// get a dedicated explanation instead of "unknown field".
var outcomeFields = map[string]string{
	"hit_rate":        "cache hit rate comes from workload.keys (distribution and count) and the cache's memory",
	"hit_ratio":       "cache hit rate comes from workload.keys (distribution and count) and the cache's memory",
	"miss_rate":       "cache hit rate comes from workload.keys (distribution and count) and the cache's memory",
	"max_rps":         "capacity comes from resources and the work each request does (cpu_ms, calls)",
	"capacity":        "capacity comes from resources and the work each request does (cpu_ms, calls)",
	"max_throughput":  "capacity comes from resources and the work each request does (cpu_ms, calls)",
	"fails_at":        "the failure point is what the run measures",
	"failure_point":   "the failure point is what the run measures",
	"breaking_point":  "the failure point is what the run measures",
	"saturation":      "saturation comes from resources and the work each request does",
	"latency_ms":      "latency comes from the work each request does and the load; use scenario faults to add latency on purpose",
	"p99_ms":          "latency is measured; write predictions about it under expect",
	"error_rate":      "errors are measured; write predictions about them under expect",
	"lag_ms":          "replication lag comes from write load and replica resources",
	"replication_lag": "replication lag comes from write load and replica resources",
}

// plannedFields are documented for a later phase but not accepted yet.
var plannedFields = map[string]string{
	"cloud":      "cloud mode is planned for phase 2",
	"partitions": "queues (kafka, rabbitmq) are planned for phase 2",
	"retention":  "queues (kafka, rabbitmq) are planned for phase 2",
}

// plannedValues are enum values documented for a later phase.
var plannedValues = map[string]map[string]string{
	"kind": {
		"kafka":    "queues (kafka, rabbitmq) are planned for phase 2",
		"rabbitmq": "queues (kafka, rabbitmq) are planned for phase 2",
	},
}

type nodePos struct{ key, val *yaml.Node }

type decoder struct {
	diags Diagnostics
	nodes map[string]nodePos
	// failed holds paths whose value was rejected while decoding, so
	// validation doesn't report a second, confusing error about them.
	failed map[string]bool
}

func newDecoder() *decoder {
	return &decoder{nodes: map[string]nodePos{}, failed: map[string]bool{}}
}

func (d *decoder) add(sev Severity, path string, n *yaml.Node, col int, code, msg string) {
	diag := Diagnostic{Severity: sev, Path: path, Code: code, Message: msg}
	if n != nil {
		diag.Line, diag.Column = n.Line, n.Column
		if col > 0 {
			diag.Column = col
		}
	}
	d.diags = append(d.diags, diag)
	if sev == SevError {
		d.failed[path] = true
	}
}

func (d *decoder) errorf(path string, n *yaml.Node, code, format string, args ...any) {
	d.add(SevError, path, n, 0, code, fmt.Sprintf(format, args...))
}

// decodeDocument decodes a YAML document into the Design.
func (d *decoder) decodeDocument(data []byte, out *Design) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		d.diags = append(d.diags, syntaxDiagnostic(err))
		return
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		d.diags = append(d.diags, Diagnostic{Severity: SevError, Code: CodeSyntax, Message: "the file is empty"})
		return
	}
	root := doc.Content[0]
	d.nodes[""] = nodePos{val: root}
	d.decode(root, reflect.ValueOf(out).Elem(), "", tagInfo{})
}

var yamlLineRe = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)

func syntaxDiagnostic(err error) Diagnostic {
	msg := err.Error()
	diag := Diagnostic{Severity: SevError, Code: CodeSyntax, Message: msg}
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
		diag.Line, _ = strconv.Atoi(m[1])
		diag.Message = "invalid YAML: " + m[2]
	} else {
		diag.Message = "invalid YAML: " + strings.TrimPrefix(msg, "yaml: ")
	}
	return diag
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

var unmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

// shorthand is a struct that can also be written as one scalar, which sets
// the named field: a pool's size, say.
type shorthand interface{ shorthandField() string }

func (d *decoder) decode(n *yaml.Node, v reflect.Value, path string, tag tagInfo) {
	n = resolveAlias(n)
	if isNull(n) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
			return // absent; defaults and required checks handle it
		}
		d.errorf(path, n, CodeType, "expected %s, found an empty value", describeType(v.Type()))
		return
	}

	if v.CanAddr() && v.Addr().Type().Implements(unmarshalerType) {
		err := v.Addr().Interface().(yaml.Unmarshaler).UnmarshalYAML(n)
		if err == nil {
			return
		}
		var se *ExpectSyntaxError
		if errors.As(err, &se) {
			d.add(SevError, path, n, n.Column+se.Offset+quoteWidth(n), CodeExpectSyntax, err.Error())
			return
		}
		d.errorf(path, n, CodeInvalidValue, "%v", err)
		return
	}

	switch v.Kind() {
	case reflect.Pointer:
		nv := reflect.New(v.Type().Elem())
		d.decode(n, nv.Elem(), path, tag)
		v.Set(nv)
	case reflect.Struct:
		if sh, ok := v.Addr().Interface().(shorthand); ok && n.Kind == yaml.ScalarNode {
			f := structFields(v.Type()).byName[sh.shorthandField()]
			d.decode(n, v.Field(f.index), path, f.tag)
			return
		}
		d.decodeStruct(n, v, path)
	case reflect.Map:
		d.decodeMap(n, v, path, tag)
	case reflect.Slice:
		d.decodeSlice(n, v, path, tag)
	default:
		d.decodeScalar(n, v, path, tag)
	}
}

// quoteWidth is the number of characters before a scalar's content starts.
func quoteWidth(n *yaml.Node) int {
	if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0 {
		return 1
	}
	return 0
}

func (d *decoder) decodeStruct(n *yaml.Node, v reflect.Value, path string) {
	if n.Kind != yaml.MappingNode {
		d.errorf(path, n, CodeType, "expected a mapping of fields, found %s", describeNode(n))
		return
	}
	fields := structFields(v.Type())
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		name := k.Value
		fpath := pathKey(path, name)
		if seen[name] {
			d.errorf(fpath, k, CodeDuplicateKey, "%q is set twice", name)
			continue
		}
		seen[name] = true
		f, ok := fields.byName[name]
		if !ok {
			d.unknownField(fpath, k, name, fields.names)
			continue
		}
		d.nodes[fpath] = nodePos{key: k, val: val}
		d.decode(val, v.Field(f.index), fpath, f.tag)
	}
	for _, f := range fields.list {
		if f.tag.required && !seen[f.name] {
			d.errorf(pathKey(path, f.name), n, CodeRequired, "missing required field %q", f.name)
		}
	}
}

func (d *decoder) unknownField(path string, k *yaml.Node, name string, allowed []string) {
	if why, ok := outcomeFields[name]; ok {
		d.errorf(path, k, CodeOutcomeField,
			"%q would set an outcome, and outcomes aren't configurable: they emerge from the run (%s)", name, why)
		return
	}
	if why, ok := plannedFields[name]; ok {
		d.errorf(path, k, CodePlanned, "%q isn't supported in this version: %s", name, why)
		return
	}
	msg := fmt.Sprintf("unknown field %q", name)
	if s := suggest(name, allowed); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}
	d.errorf(path, k, CodeUnknownField, "%s", msg)
}

func (d *decoder) decodeMap(n *yaml.Node, v reflect.Value, path string, tag tagInfo) {
	if n.Kind != yaml.MappingNode {
		d.errorf(path, n, CodeType, "expected a mapping, found %s", describeNode(n))
		return
	}
	if v.IsNil() {
		v.Set(reflect.MakeMap(v.Type()))
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		fpath := pathKey(path, k.Value)
		if k.Kind != yaml.ScalarNode {
			d.errorf(path, k, CodeType, "map keys must be plain names")
			continue
		}
		if seen[k.Value] {
			d.errorf(fpath, k, CodeDuplicateKey, "%q is set twice", k.Value)
			continue
		}
		seen[k.Value] = true
		d.nodes[fpath] = nodePos{key: k, val: val}
		elem := reflect.New(v.Type().Elem()).Elem()
		d.decode(val, elem, fpath, tagInfo{})
		v.SetMapIndex(reflect.ValueOf(k.Value), elem)
	}
	if tag.minProperties > 0 && len(seen) < tag.minProperties {
		d.errorf(path, n, CodeRequired, "needs at least %d entr%s", tag.minProperties, plural(tag.minProperties, "y", "ies"))
	}
}

func (d *decoder) decodeSlice(n *yaml.Node, v reflect.Value, path string, tag tagInfo) {
	if n.Kind != yaml.SequenceNode {
		d.errorf(path, n, CodeType, "expected a list, found %s", describeNode(n))
		return
	}
	s := reflect.MakeSlice(v.Type(), 0, len(n.Content))
	for i, item := range n.Content {
		ipath := pathIndex(path, i)
		d.nodes[ipath] = nodePos{val: item}
		elem := reflect.New(v.Type().Elem()).Elem()
		d.decode(item, elem, ipath, tagInfo{enum: tag.enum})
		s = reflect.Append(s, elem)
	}
	v.Set(s)
	if tag.minItems != nil && len(n.Content) < *tag.minItems {
		d.errorf(path, n, CodeRange, "needs at least %d item%s", *tag.minItems, plural(*tag.minItems, "", "s"))
	}
	if tag.maxItems != nil && len(n.Content) > *tag.maxItems {
		d.errorf(path, n, CodeRange, "takes at most %d item%s", *tag.maxItems, plural(*tag.maxItems, "", "s"))
	}
}

func (d *decoder) decodeScalar(n *yaml.Node, v reflect.Value, path string, tag tagInfo) {
	if n.Kind != yaml.ScalarNode {
		d.errorf(path, n, CodeType, "expected %s, found %s", describeType(v.Type()), describeNode(n))
		return
	}
	if v.Kind() == reflect.String && n.ShortTag() != "!!str" && n.ShortTag() != "!!int" && n.ShortTag() != "!!float" {
		// Booleans and other tags become strings only when quoted.
		d.errorf(path, n, CodeType, "expected a string, found %s", describeNode(n))
		return
	}
	if (v.Kind() == reflect.Int || v.Kind() == reflect.Int64) && n.ShortTag() != "!!int" {
		d.errorf(path, n, CodeType, "expected %s, found %s", describeType(v.Type()), describeNode(n))
		return
	}
	if err := n.Decode(v.Addr().Interface()); err != nil {
		d.errorf(path, n, CodeType, "expected %s, found %q", describeType(v.Type()), n.Value)
		return
	}
	name := lastSegment(path)
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if len(tag.enum) > 0 && !slices.Contains(tag.enum, s) {
			if why, ok := plannedValues[name][s]; ok {
				d.errorf(path, n, CodePlanned, "%q isn't supported in this version: %s", s, why)
				return
			}
			msg := fmt.Sprintf("%q isn't valid; use one of: %s", s, strings.Join(tag.enum, ", "))
			if sg := suggest(s, tag.enum); sg != "" {
				msg = fmt.Sprintf("%q isn't valid (did you mean %q?); use one of: %s", s, sg, strings.Join(tag.enum, ", "))
			}
			d.errorf(path, n, CodeInvalidValue, "%s", msg)
			return
		}
		if tag.pattern != nil && !tag.pattern.MatchString(s) {
			d.errorf(path, n, CodeInvalidValue, "%q isn't valid: %s", s, tag.patternHint)
		}
		if tag.maxLength > 0 && len(s) > tag.maxLength {
			d.errorf(path, n, CodeRange, "is longer than %d characters", tag.maxLength)
		}
	case reflect.Int, reflect.Int64:
		d.checkRange(path, n, float64(v.Int()), tag)
		if len(tag.enum) > 0 && !slices.Contains(tag.enum, strconv.FormatInt(v.Int(), 10)) {
			d.errorf(path, n, CodeInvalidValue, "must be %s", strings.Join(tag.enum, " or "))
		}
	case reflect.Float64:
		d.checkRange(path, n, v.Float(), tag)
	}
}

func (d *decoder) checkRange(path string, n *yaml.Node, f float64, tag tagInfo) {
	switch {
	case tag.minimum != nil && f < *tag.minimum:
		d.errorf(path, n, CodeRange, "must be at least %s, found %s", fmtNum(*tag.minimum), n.Value)
	case tag.exclusiveMinimum != nil && f <= *tag.exclusiveMinimum:
		d.errorf(path, n, CodeRange, "must be greater than %s, found %s", fmtNum(*tag.exclusiveMinimum), n.Value)
	case tag.maximum != nil && f > *tag.maximum:
		d.errorf(path, n, CodeRange, "must be at most %s, found %s", fmtNum(*tag.maximum), n.Value)
	}
}

func lastSegment(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.IndexByte(path, '['); i >= 0 {
		path = path[:i]
	}
	return path
}

func describeNode(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	}
	switch n.ShortTag() {
	case "!!bool":
		return "a boolean"
	case "!!int", "!!float":
		return "a number"
	}
	return strconv.Quote(n.Value)
}

func describeType(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int64:
		return "a whole number"
	case reflect.Float64:
		return "a number"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "a mapping"
	}
	return t.String()
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// --- struct tags ---

type tagInfo struct {
	required         bool
	enum             []string
	minimum          *float64
	maximum          *float64
	exclusiveMinimum *float64
	minItems         *int
	maxItems         *int
	minProperties    int
	maxLength        int
	pattern          *regexp.Regexp
	patternHint      string
}

type fieldInfo struct {
	name  string
	index int
	tag   tagInfo
}

type structInfo struct {
	list   []fieldInfo
	byName map[string]fieldInfo
	names  []string
}

var structCache sync.Map // reflect.Type -> *structInfo

func structFields(t reflect.Type) *structInfo {
	if si, ok := structCache.Load(t); ok {
		return si.(*structInfo)
	}
	si := &structInfo{byName: map[string]fieldInfo{}}
	for i := range t.NumField() {
		sf := t.Field(i)
		name, _, _ := strings.Cut(sf.Tag.Get("yaml"), ",")
		if name == "" || name == "-" || !sf.IsExported() {
			continue
		}
		f := fieldInfo{name: name, index: i, tag: parseTag(sf.Tag.Get("jsonschema"))}
		si.list = append(si.list, f)
		si.byName[name] = f
		si.names = append(si.names, name)
	}
	structCache.Store(t, si)
	return si
}

func parseTag(s string) tagInfo {
	var ti tagInfo
	if s == "" {
		return ti
	}
	num := func(v string) *float64 {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			panic(fmt.Sprintf("design: bad number %q in jsonschema tag %q", v, s))
		}
		return &f
	}
	for part := range strings.SplitSeq(s, ",") {
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "required":
			ti.required = true
		case "enum":
			ti.enum = append(ti.enum, v)
		case "minimum":
			ti.minimum = num(v)
		case "maximum":
			ti.maximum = num(v)
		case "exclusiveMinimum":
			ti.exclusiveMinimum = num(v)
		case "minItems":
			n := int(*num(v))
			ti.minItems = &n
		case "maxItems":
			n := int(*num(v))
			ti.maxItems = &n
		case "minProperties":
			ti.minProperties = int(*num(v))
		case "maxLength":
			ti.maxLength = int(*num(v))
		case "pattern":
			ti.pattern = regexp.MustCompile(v)
			ti.patternHint = "use lowercase letters, digits and '-', such as my-design"
		case "default":
		default:
			panic(fmt.Sprintf("design: unsupported jsonschema tag key %q in %q", k, s))
		}
	}
	return ti
}

// suggest returns the closest candidate within edit distance 2, or "".
func suggest(s string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := editDistance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
