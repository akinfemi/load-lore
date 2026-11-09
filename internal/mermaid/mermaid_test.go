package mermaid

import "testing"

func TestID(t *testing.T) {
	for in, want := range map[string]string{
		"api": "api", "orders-db": "orders_db", "end": "n_end", "Class": "n_Class", "1st": "n_1st", "": "n_",
	} {
		if got := ID(in); got != want {
			t.Errorf("ID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlowchart(t *testing.T) {
	f := New(LeftRight)
	f.Node("load", Stadium, "load", "up to 5,000 rps")
	f.Node("api", Box, `api "edge" <x>`)
	f.Node("db", Cylinder, "db")
	f.Node("pgb", Rounded, "pgb")
	f.Node("api", Box, "api ×2") // replaces the label, keeps the position
	f.Edge("load", "api", Solid, "90% read")
	f.Edge("api", "db", Dotted, "read ×3", "pool 24")
	f.Edge("db", "pgb", Solid)
	f.ClassDef("hot", "fill:#f8d7da,stroke:#dc3545")
	f.Class("db", "hot")
	f.Class("pgb", "hot")

	want := `flowchart LR
    load(["load<br/>up to 5,000 rps"])
    api["api ×2"]
    db[("db")]
    pgb("pgb")
    load -->|"90% read"| api
    api -.->|"read ×3<br/>pool 24"| db
    db --> pgb
    classDef hot fill:#f8d7da,stroke:#dc3545
    class db,pgb hot
`
	if got := f.String(); got != want {
		t.Errorf("String():\n%s\nwant:\n%s", got, want)
	}

	plainWant := "graph LR\nload[load<br/>up to 5,000 rps]\napi[api ×2]\ndb[db]\npgb[pgb]\n" +
		"load -->|90% read| api\napi -.->|read ×3 · pool 24| db\ndb --> pgb\n"
	if got := f.Plain(); got != plainWant {
		t.Errorf("Plain():\n%q\nwant:\n%q", got, plainWant)
	}
}

func TestEscaping(t *testing.T) {
	f := New(TopDown)
	f.Node("a", Box, `say "hi" <b>`, "x|y [z]")
	f.Edge("a", "a", Solid, `"q"`)
	want := "flowchart TD\n    a[\"say #quot;hi#quot; #lt;b#gt;<br/>x|y [z]\"]\n    a -->|\"#quot;q#quot;\"| a\n"
	if got := f.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	plainWant := "graph TD\na[say hi b<br/>x/y z]\na -->|q| a\n"
	if got := f.Plain(); got != plainWant {
		t.Errorf("plain got:\n%q\nwant:\n%q", got, plainWant)
	}
}
