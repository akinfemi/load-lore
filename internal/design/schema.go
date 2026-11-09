package design

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// SchemaID identifies the generated schema.
const SchemaID = "https://github.com/akinfemi/load-lore/schema/design-v1.json"

// JSONSchema generates the design file's JSON Schema from the Go types.
func JSONSchema() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		FieldNameTag:               "yaml",
		RequiredFromJSONSchemaTags: true,
		DoNotReference:             false,
		ExpandedStruct:             true,
	}
	s := r.Reflect(&Design{})
	s.ID = SchemaID
	s.Title = "Loadlore design file"
	s.Description = "One experiment: the system (services), how it is loaded (workload), what goes wrong (scenario), and what the learner expects (expect)."

	// Service names must match the same pattern validation enforces.
	if svc, ok := s.Properties.Get("services"); ok {
		svc.PropertyNames = &jsonschema.Schema{Pattern: serviceNameRe.String(), MaxLength: ptr(uint64(63))}
	}
	return s
}

// JSONSchemaBytes returns the schema as indented JSON.
func JSONSchemaBytes() ([]byte, error) {
	return json.MarshalIndent(JSONSchema(), "", "  ")
}
