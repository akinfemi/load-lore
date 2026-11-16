// Package predict turns answers to a scenario's questions into expect
// lines, and writes them into a design file without touching the rest of
// it.
package predict

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// Line is the expect line an answer to q becomes. services are the
// design's service names, for service answers.
func Line(q scenario.Question, answer string, services []string) (string, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("an answer is needed")
	}
	var v string
	switch q.Answer {
	case scenario.AnswerRPS:
		if strings.EqualFold(answer, "never") {
			if q.Never == "" {
				return "", fmt.Errorf("give a load in requests per second")
			}
			return q.Never, nil
		}
		n, err := parseRPS(answer)
		if err != nil {
			return "", err
		}
		v = strconv.FormatFloat(n, 'f', -1, 64)
	case scenario.AnswerService:
		i := slices.IndexFunc(services, func(s string) bool { return strings.EqualFold(s, answer) })
		if i < 0 {
			return "", fmt.Errorf("%q isn't one of the design's services: %s", answer, strings.Join(services, ", "))
		}
		v = services[i]
	case scenario.AnswerYesNo:
		switch strings.ToLower(strings.TrimSuffix(answer, ".")) {
		case "yes", "y":
			return q.Expect, nil
		case "no", "n":
			return q.No, nil
		}
		return "", fmt.Errorf("answer yes or no")
	case scenario.AnswerText:
		// Answers go inside a quoted note, so a double quote would end it.
		v = strings.ReplaceAll(strings.Join(strings.Fields(answer), " "), `"`, "'")
	default:
		return "", fmt.Errorf("unknown answer kind %q", q.Answer)
	}
	return strings.ReplaceAll(q.Expect, "{answer}", v), nil
}

// parseRPS reads a load such as 1000, 1,000, 1000 rps or 1.5k.
func parseRPS(s string) (float64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimSpace(strings.TrimSuffix(t, "rps"))
	t = strings.ReplaceAll(t, ",", "")
	mult := 1.0
	if strings.HasSuffix(t, "k") {
		mult, t = 1000, strings.TrimSuffix(t, "k")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q isn't a load: give requests per second, such as 1000, or never", s)
	}
	return n * mult, nil
}

var expectKey = regexp.MustCompile(`^expect:\s*(\[\s*\])?\s*(#.*)?$`)

// Rewrite replaces the expect block of a design file's source with lines,
// keeping everything else byte for byte, and checks that the result
// still loads. Without an expect block, it adds one at the end.
func Rewrite(src []byte, lines []string) ([]byte, error) {
	var block strings.Builder
	block.WriteString("expect:\n")
	for _, l := range lines {
		block.WriteString("  - " + l + "\n")
	}
	text := string(src)
	rows := strings.SplitAfter(text, "\n")
	start := slices.IndexFunc(rows, func(r string) bool { return expectKey.MatchString(strings.TrimRight(r, "\r\n")) })
	var out string
	if start < 0 {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		out = text + "\n# Your predictions, scored in the report.\n" + block.String()
	} else {
		// The block runs on while lines are indented or list items; blank
		// lines count only when more of the block follows them.
		end := start + 1
		for i := start + 1; i < len(rows); i++ {
			r := strings.TrimRight(rows[i], "\r\n")
			if strings.TrimSpace(r) == "" {
				continue
			}
			if r[0] == ' ' || r[0] == '\t' || r[0] == '-' {
				end = i + 1
				continue
			}
			break
		}
		out = strings.Join(rows[:start], "") + block.String() + strings.Join(rows[end:], "")
	}
	if _, diags := design.Load([]byte(out)); diags.HasErrors() {
		return nil, fmt.Errorf("the design wouldn't load with these predictions: %s", diags.Errors()[0].Message)
	}
	return []byte(out), nil
}
