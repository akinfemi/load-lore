package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// promClient queries the run's Prometheus through its published port.
type promClient struct {
	base string
	hc   *http.Client
}

func newPromClient(addr string) *promClient {
	return &promClient{base: "http://" + addr, hc: &http.Client{Timeout: 30 * time.Second}}
}

type promResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
}

func (c *promClient) get(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr promResponse
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil, fmt.Errorf("prometheus %s: %s", path, strings.TrimSpace(string(b)))
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus %s: %s: %s", path, pr.ErrorType, pr.Error)
	}
	return pr.Data, nil
}

// scalar runs an instant query whose result is one number. ok is false
// when the query returns no data.
func (c *promClient) scalar(ctx context.Context, query string) (float64, bool, error) {
	data, err := c.get(ctx, "/api/v1/query", url.Values{"query": {query}})
	if err != nil {
		return 0, false, err
	}
	var v struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, false, err
	}
	if len(v.Result) == 0 {
		return 0, false, nil
	}
	s, _ := v.Result[0].Value[1].(string)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, nil
	}
	return f, true, nil
}

// vector runs an instant query and returns each series' value by one label.
func (c *promClient) vector(ctx context.Context, query, label string) (map[string]float64, error) {
	data, err := c.get(ctx, "/api/v1/query", url.Values{"query": {query}})
	if err != nil {
		return nil, err
	}
	var v struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, r := range v.Result {
		s, _ := r.Value[1].(string)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			out[r.Metric[label]] = f
		}
	}
	return out, nil
}

// exportedMetrics selects which series a run keeps.
var exportedMetrics = regexp.MustCompile(`^(loadlore_|container_|pg_|redis_|k6_|toxiproxy_|up$|scrape_duration_seconds$)`)

// export writes every kept series over [from, to] at one-second steps to
// dir/<metric>.json, as Prometheus range-query results. Native histograms
// come out as the histograms Prometheus stores.
func (c *promClient) export(ctx context.Context, from, to time.Time, write func(name string, data []byte) error) (int, error) {
	data, err := c.get(ctx, "/api/v1/label/__name__/values", url.Values{
		"start": {fmtTime(from)}, "end": {fmtTime(to)},
	})
	if err != nil {
		return 0, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		if !exportedMetrics.MatchString(name) {
			continue
		}
		res, err := c.get(ctx, "/api/v1/query_range", url.Values{
			"query": {name}, "start": {fmtTime(from)}, "end": {fmtTime(to)}, "step": {"1"},
		})
		if err != nil {
			return n, fmt.Errorf("exporting %s: %w", name, err)
		}
		if err := write(name+".json", res); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func fmtTime(t time.Time) string { return strconv.FormatFloat(float64(t.UnixMilli())/1000, 'f', 3, 64) }
