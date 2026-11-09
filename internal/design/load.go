package design

import (
	"fmt"
	"os"
)

// Load parses a design file, applies defaults and validates it. It returns
// the design (possibly partial when there are errors) and every diagnostic,
// sorted by position.
func Load(data []byte) (*Design, Diagnostics) {
	d := &Design{}
	dec := newDecoder()
	dec.decodeDocument(data, d)
	if len(dec.nodes) == 0 { // not even a document: syntax error or empty
		return d, dec.diags.sorted()
	}
	applyDefaults(d)
	v := &validator{d: d, nodes: dec.nodes, failed: dec.failed}
	v.validate()
	return d, append(dec.diags, v.diags...).sorted()
}

// LoadFile reads and loads a design file. The error is for I/O problems only;
// problems with the design are diagnostics.
func LoadFile(path string) (*Design, Diagnostics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading design: %w", err)
	}
	d, diags := Load(data)
	return d, diags, nil
}
