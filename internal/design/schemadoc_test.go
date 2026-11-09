package design

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The skill's schema.md and `loadl schema` must describe the same fields.
// Forward: every YAML key in schema.md's examples exists at that position in
// the schema, and every backticked name in a table's first column is a schema
// field or enum value. Reverse: every schema field name appears in schema.md.
// Everything from the "## Planned" heading on is skipped.

const schemaDoc = "../../skills/loadlore-designer/references/schema.md"

type schemaNode = map[string]any

func loadSchemaMap(t *testing.T) schemaNode {
	t.Helper()
	b, err := JSONSchemaBytes()
	if err != nil {
		t.Fatal(err)
	}
	var m schemaNode
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func loadSchemaDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(schemaDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	if i := strings.Index(doc, "\n## Planned"); i >= 0 {
		doc = doc[:i]
	}
	return doc
}

// resolve follows $ref into $defs.
func resolve(root, s schemaNode) schemaNode {
	for {
		ref, ok := s["$ref"].(string)
		if !ok {
			return s
		}
		s = root["$defs"].(schemaNode)[strings.TrimPrefix(ref, "#/$defs/")].(schemaNode)
	}
}

// child returns the schema for key under s; ok is false when s has no such
// property. leaf is true when s accepts anything below (free maps, oneOf).
func child(root, s schemaNode, key string) (next schemaNode, ok, leaf bool) {
	s = resolve(root, s)
	if _, isOneOf := s["oneOf"]; isOneOf {
		return nil, true, true
	}
	if props, has := s["properties"].(schemaNode); has {
		if p, found := props[key].(schemaNode); found {
			return p, true, false
		}
	}
	if ap, has := s["additionalProperties"].(schemaNode); has {
		return ap, true, false
	}
	return nil, false, false
}

type yamlBlock struct {
	line int
	text string
}

var fenceRe = regexp.MustCompile("(?s)```yaml\n(.*?)```")

func yamlBlocks(doc string) []yamlBlock {
	var out []yamlBlock
	for _, m := range fenceRe.FindAllStringSubmatchIndex(doc, -1) {
		out = append(out, yamlBlock{
			line: strings.Count(doc[:m[2]], "\n") + 1,
			text: doc[m[2]:m[3]],
		})
	}
	return out
}

// walkYAML checks every mapping key in n against schema s, collecting the
// names it saw and the keys the schema lacks.
func walkYAML(root, s schemaNode, n *yaml.Node, path string, seen map[string]bool, missing *[]string) {
	s = resolve(root, s)
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			next, ok, leaf := child(root, s, k)
			if !ok {
				*missing = append(*missing, pathKey(path, k))
				continue
			}
			seen[k] = true
			if !leaf {
				walkYAML(root, next, v, pathKey(path, k), seen, missing)
			}
		}
	case yaml.SequenceNode:
		items, ok := s["items"].(schemaNode)
		if !ok {
			return
		}
		for i, item := range n.Content {
			walkYAML(root, items, item, pathIndex(path, i), seen, missing)
		}
	}
}

func TestSchemaDocYAMLKeysExistInSchema(t *testing.T) {
	root := loadSchemaMap(t)
	doc := loadSchemaDoc(t)
	defs := root["$defs"].(schemaNode)
	// A snippet may be a whole design, the value of `services`, or one service.
	roots := map[string]schemaNode{
		"design":   root,
		"services": root["properties"].(schemaNode)["services"].(schemaNode),
		"service":  defs["Service"].(schemaNode),
	}
	for _, b := range yamlBlocks(doc) {
		var n yaml.Node
		if err := yaml.Unmarshal([]byte(b.text), &n); err != nil {
			t.Errorf("schema.md:%d: YAML example doesn't parse: %v", b.line, err)
			continue
		}
		body := n.Content[0]
		// Use the first root that knows every top-level key of the snippet.
		var best []string
		matched := false
		for _, name := range []string{"design", "service", "services"} {
			var missing []string
			walkYAML(root, roots[name], body, "", map[string]bool{}, &missing)
			if !slices.ContainsFunc(missing, func(p string) bool { return !strings.ContainsAny(p, ".[") }) {
				best, matched = missing, true
				break
			}
		}
		if !matched {
			t.Errorf("schema.md:%d: example's top-level keys don't match a design, a services map or a service", b.line)
		}
		for _, m := range best {
			t.Errorf("schema.md:%d: example uses %q, which isn't in `loadl schema`", b.line, m)
		}
	}
}

// schemaNames collects every property name and string enum value.
func schemaNames(root schemaNode) (fields, enums map[string]bool) {
	fields, enums = map[string]bool{}, map[string]bool{}
	var walk func(s schemaNode)
	visited := map[string]bool{}
	walk = func(s schemaNode) {
		if ref, ok := s["$ref"].(string); ok {
			if visited[ref] {
				return
			}
			visited[ref] = true
			walk(resolve(root, s))
			return
		}
		if props, ok := s["properties"].(schemaNode); ok {
			for k, v := range props {
				fields[k] = true
				walk(v.(schemaNode))
			}
		}
		if e, ok := s["enum"].([]any); ok {
			for _, v := range e {
				if str, ok := v.(string); ok {
					enums[str] = true
				}
			}
		}
		for _, key := range []string{"items", "additionalProperties"} {
			if c, ok := s[key].(schemaNode); ok {
				walk(c)
			}
		}
		if alts, ok := s["oneOf"].([]any); ok {
			for _, a := range alts {
				walk(a.(schemaNode))
			}
		}
	}
	walk(root)
	return fields, enums
}

var (
	tableFirstCell = regexp.MustCompile(`(?m)^\|\s*([^|]*?)\s*\|`)
	backticked     = regexp.MustCompile("`([^`]+)`")
	identRe        = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

func TestSchemaDocTableNamesExistInSchema(t *testing.T) {
	fields, enums := schemaNames(loadSchemaMap(t))
	doc := regexp.MustCompile("(?s)```.*?```").ReplaceAllString(loadSchemaDoc(t), "")
	for _, m := range tableFirstCell.FindAllStringSubmatch(doc, -1) {
		for _, bt := range backticked.FindAllStringSubmatch(m[1], -1) {
			for seg := range strings.SplitSeq(bt[1], ".") {
				if !identRe.MatchString(seg) {
					continue // a value such as `1` or `512m`
				}
				if !fields[seg] && !enums[seg] {
					t.Errorf("schema.md table names `%s`, which isn't a field or value in `loadl schema`", bt[1])
				}
			}
		}
	}
}

func TestSchemaFieldsAreDocumented(t *testing.T) {
	fields, _ := schemaNames(loadSchemaMap(t))
	doc := loadSchemaDoc(t)
	documented := map[string]bool{}
	prose := regexp.MustCompile("(?s)```.*?```").ReplaceAllString(doc, "")
	for _, m := range backticked.FindAllStringSubmatch(prose, -1) {
		for _, w := range regexp.MustCompile(`[a-z][a-z0-9_]*`).FindAllString(m[1], -1) {
			documented[w] = true
		}
	}
	for _, b := range yamlBlocks(doc) {
		for _, w := range regexp.MustCompile(`(?m)([a-z][a-z0-9_]*)\s*:`).FindAllStringSubmatch(b.text, -1) {
			documented[w[1]] = true
		}
	}
	var undocumented []string
	for f := range fields {
		if !documented[f] {
			undocumented = append(undocumented, f)
		}
	}
	slices.Sort(undocumented)
	for _, f := range undocumented {
		t.Errorf("schema field %q isn't documented in schema.md", f)
	}
}
