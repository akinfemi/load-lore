package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// toxiproxyClient talks to the Toxiproxy API published on the host.
type toxiproxyClient struct {
	base string
	hc   *http.Client
}

type toxic struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Stream     string         `json:"stream"`
	Toxicity   float64        `json:"toxicity"`
	Attributes map[string]int `json:"attributes"`
}

func newToxiproxyClient(addr string) *toxiproxyClient {
	return &toxiproxyClient{base: "http://" + addr, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (t *toxiproxyClient) addToxic(ctx context.Context, proxy string, x toxic) error {
	body, _ := json.Marshal(x)
	return t.do(ctx, http.MethodPost, "/proxies/"+proxy+"/toxics", body)
}

func (t *toxiproxyClient) removeToxic(ctx context.Context, proxy, name string) error {
	return t.do(ctx, http.MethodDelete, "/proxies/"+proxy+"/toxics/"+name, nil)
}

func (t *toxiproxyClient) ping(ctx context.Context) error {
	return t.do(ctx, http.MethodGet, "/version", nil)
}

func (t *toxiproxyClient) do(ctx context.Context, method, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("toxiproxy %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(b))
	}
	return nil
}
