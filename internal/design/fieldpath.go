package design

import (
	"fmt"
	"reflect"
	"slices"
)

// cursor walks the design's types along a field path. v is the value at
// that point, or invalid when the field isn't set (the path can still name a
// defaulted field). free means anything below is accepted (native config).
type cursor struct {
	t    reflect.Type
	v    reflect.Value
	free bool
}

func rootCursor(d *Design) cursor {
	return cursor{t: reflect.TypeFor[Design](), v: reflect.ValueOf(d).Elem()}
}

var serviceMapType = reflect.TypeFor[map[string]*Service]()

func (c cursor) step(seg pathSeg) (cursor, error) {
	t, v := c.t, c.v
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if v.IsValid() {
			if v.IsNil() {
				v = reflect.Value{}
			} else {
				v = v.Elem()
			}
		}
	}

	if seg.index >= 0 {
		if t.Kind() != reflect.Slice {
			return c, fmt.Errorf("[%d] used on something that isn't a list", seg.index)
		}
		next := cursor{t: t.Elem()}
		if v.IsValid() {
			if seg.index >= v.Len() {
				return c, fmt.Errorf("index %d is out of range (the list has %d item%s)", seg.index, v.Len(), plural(v.Len(), "", "s"))
			}
			next.v = v.Index(seg.index)
		}
		return next, nil
	}

	switch t.Kind() {
	case reflect.Struct:
		if reflect.PointerTo(t).Implements(unmarshalerType) {
			return c, fmt.Errorf("%q is a value, not a mapping with fields", seg.name)
		}
		si := structFields(t)
		f, ok := si.byName[seg.name]
		if !ok {
			if s := suggest(seg.name, si.names); s != "" {
				return c, fmt.Errorf("no field %q (did you mean %q?)", seg.name, s)
			}
			return c, fmt.Errorf("no field %q", seg.name)
		}
		next := cursor{t: t.Field(f.index).Type}
		if v.IsValid() {
			next.v = v.Field(f.index)
		}
		return next, nil
	case reflect.Map:
		if t != serviceMapType {
			return cursor{free: true}, nil
		}
		if v.IsValid() {
			e := v.MapIndex(reflect.ValueOf(seg.name))
			if !e.IsValid() {
				names := sortedKeys(v.Interface().(map[string]*Service))
				if s := suggest(seg.name, names); s != "" {
					return c, fmt.Errorf("no service %q (did you mean %q?)", seg.name, s)
				}
				return c, fmt.Errorf("no service %q", seg.name)
			}
			return cursor{t: t.Elem(), v: e}, nil
		}
		return cursor{t: t.Elem()}, nil
	}
	return c, fmt.Errorf("%q: the value before it has no fields", seg.name)
}

func sortedKeys[K interface{ ~string | ~int64 }, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func mapKeys[V any](m map[string]V) []string { return sortedKeys(m) }
