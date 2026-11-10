// Package scenario reads scenario folders: a starting design, its story,
// hints, improved designs and the claims `loadl scenario check` verifies.
//
//	<name>/
//	  README.md      the story, the question, what to try, the diagram
//	  design.yaml    the starting design; its name is the folder's
//	  hints.md       progressive hints, one "## " section each
//	  meta.yaml      level, concepts, time, host resources, check claims
//	  solutions/<solution>/
//	    README.md    what changed and why
//	    design.yaml  the improved design
//	    report.md    a reference run's report (optional)
package scenario

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/akinfemi/load-lore/internal/design"
	library "github.com/akinfemi/load-lore/scenarios"
)

// Files in a scenario folder.
const (
	MetaFile       = "meta.yaml"
	DesignFile     = "design.yaml"
	ReadmeFile     = "README.md"
	HintsFile      = "hints.md"
	SolutionsDir   = "solutions"
	ReportFile     = "report.md"
	DefaultMinGain = 1.2
)

// Scenario is one scenario folder, on disk or built into loadl.
type Scenario struct {
	Name      string   `json:"name"`
	Path      string   `json:"path,omitempty"` // empty for the built-in copy
	Meta      Meta     `json:"meta"`
	Hints     []Hint   `json:"-"` // revealed one at a time by loadl hint
	HintCount int      `json:"hints"`
	Solutions []string `json:"solutions"`

	fsys fs.FS
}

// Meta is meta.yaml.
type Meta struct {
	Title    string   `yaml:"title" json:"title"`
	Level    int      `yaml:"level" json:"level"`
	Concepts []string `yaml:"concepts" json:"concepts"`
	Minutes  int      `yaml:"minutes" json:"minutes"` // rough time to work through it
	Host     Host     `yaml:"host" json:"host"`
	Check    Claims   `yaml:"check" json:"check"`
}

// Host is what Docker must have available for the scenario at --scale 1,
// Loadlore's own containers included. Smaller hosts need a smaller --scale.
type Host struct {
	CPUs   float64     `yaml:"cpus" json:"cpus"`
	Memory design.Size `yaml:"memory" json:"memory_bytes"`
	// Reference is the machine the scenario was checked on, and Speed its
	// host speed index (bench.Score.Index). CPU limits are fractions of
	// the host's cores, so a slower machine breaks at a lower load.
	Reference string `yaml:"reference" json:"reference"`
	Speed     int    `yaml:"speed" json:"speed"`
}

// Claims holds the claims `loadl scenario check` verifies: the starting
// design shows the problem, and each solution helps.
type Claims struct {
	Problem   Claim            `yaml:"problem" json:"problem"`
	Solutions map[string]Claim `yaml:"solutions,omitempty" json:"solutions,omitempty"`
}

// Claim is what a run of one design must show.
type Claim struct {
	// Verdict is broke or held. Required for the problem.
	Verdict string `yaml:"verdict,omitempty" json:"verdict,omitempty"`
	// Diagnoses are rule ids that must fire.
	Diagnoses []string `yaml:"diagnoses,omitempty" json:"diagnoses,omitempty"`
	// MinGain applies to solutions when the problem broke: the solution
	// must hold, or serve at least this many times the problem's peak
	// throughput (the most successful responses per second in any
	// window). Default 1.2. Peak throughput is steadier from run to run
	// than where p99 first crosses the break threshold.
	MinGain *float64 `yaml:"min_gain,omitempty" json:"min_gain,omitempty"`
}

// Hint is one "## " section of hints.md.
type Hint struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Open reads a scenario folder on disk.
func Open(dir string) (*Scenario, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s isn't a folder", dir)
	}
	s, err := read(os.DirFS(abs), filepath.Base(abs))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	s.Path = abs
	return s, nil
}

// Library returns the scenarios built into loadl, by level then name.
func Library() ([]*Scenario, error) {
	entries, err := fs.ReadDir(library.FS, ".")
	if err != nil {
		return nil, err
	}
	var out []*Scenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(library.FS, e.Name())
		if err != nil {
			return nil, err
		}
		s, err := read(sub, e.Name())
		if err != nil {
			return nil, fmt.Errorf("built-in scenario %s: %w", e.Name(), err)
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b *Scenario) int {
		if a.Meta.Level != b.Meta.Level {
			return a.Meta.Level - b.Meta.Level
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// Find returns a scenario by name from the library, or by path when ref
// is a folder.
func Find(ref string) (*Scenario, error) {
	if st, err := os.Stat(ref); err == nil && st.IsDir() {
		return Open(ref)
	}
	lib, err := Library()
	if err != nil {
		return nil, err
	}
	for _, s := range lib {
		if s.Name == ref {
			return s, nil
		}
	}
	names := make([]string, len(lib))
	for i, s := range lib {
		names[i] = s.Name
	}
	return nil, fmt.Errorf("no scenario %q: not a folder, and the library has %s", ref, strings.Join(names, ", "))
}

// ForDesign finds the scenario a design file belongs to: its own folder
// holds meta.yaml, or it is solutions/<name>/design.yaml under one.
func ForDesign(designPath string) (*Scenario, bool) {
	abs, err := filepath.Abs(designPath)
	if err != nil {
		return nil, false
	}
	dir := filepath.Dir(abs)
	candidates := []string{dir}
	if filepath.Base(filepath.Dir(dir)) == SolutionsDir {
		candidates = append(candidates, filepath.Dir(filepath.Dir(dir)))
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, MetaFile)); err == nil {
			if s, err := Open(c); err == nil {
				return s, true
			}
		}
	}
	return nil, false
}

func read(fsys fs.FS, name string) (*Scenario, error) {
	s := &Scenario{Name: name, fsys: fsys, Solutions: []string{}}
	raw, err := fs.ReadFile(fsys, MetaFile)
	if err != nil {
		return nil, fmt.Errorf("no %s: %w", MetaFile, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&s.Meta); err != nil {
		return nil, fmt.Errorf("%s: %w", MetaFile, err)
	}
	if hints, err := fs.ReadFile(fsys, HintsFile); err == nil {
		s.Hints = ParseHints(hints)
		s.HintCount = len(s.Hints)
	}
	if entries, err := fs.ReadDir(fsys, SolutionsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				s.Solutions = append(s.Solutions, e.Name())
			}
		}
	}
	return s, nil
}

// ReadFile reads a file in the scenario folder, such as "design.yaml" or
// "solutions/x/design.yaml".
func (s *Scenario) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(s.fsys, name)
}

// SolutionDesign is the path of a solution's design inside the folder.
func SolutionDesign(name string) string { return path.Join(SolutionsDir, name, DesignFile) }

// Materialize returns the scenario's folder on disk, copying the built-in
// copy under dir first.
func (s *Scenario) Materialize(dir string) (string, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	dst := filepath.Join(dir, s.Name)
	err := fs.WalkDir(s.fsys, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dst, filepath.FromSlash(p))
		if e.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := fs.ReadFile(s.fsys, p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		return "", err
	}
	return dst, nil
}

// Matches reports whether a search query appears in the name, title or
// concepts, ignoring case. An empty query matches everything.
func (s *Scenario) Matches(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	hay := strings.ToLower(s.Name + " " + s.Meta.Title + " " + strings.Join(s.Meta.Concepts, " "))
	for _, w := range strings.Fields(q) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// ParseHints splits hints.md into its "## " sections. Text before the
// first section (a title, an intro) isn't a hint.
func ParseHints(md []byte) []Hint {
	var hints []Hint
	var body []string
	flush := func() {
		if len(hints) > 0 {
			hints[len(hints)-1].Body = strings.TrimSpace(strings.Join(body, "\n"))
		}
		body = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(md))
	fence := false
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(line, "## ") {
			flush()
			hints = append(hints, Hint{Title: strings.TrimSpace(strings.TrimPrefix(line, "## "))})
			continue
		}
		body = append(body, line)
	}
	flush()
	return hints
}
