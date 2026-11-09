package design

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	ok := map[string]time.Duration{
		"30s": 30 * time.Second, "5m": 5 * time.Minute, "1h": time.Hour,
		"1m30s": 90 * time.Second, "500ms": 500 * time.Millisecond, "0s": 0,
	}
	for in, want := range ok {
		got, err := ParseDuration(in)
		if err != nil || got.D() != want {
			t.Errorf("%q: got %v, %v; want %v", in, got.D(), err, want)
		}
	}
	for _, in := range []string{"", "30", "5 m", "1d", "-5s", "10us", "fast"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}

func TestDurationString(t *testing.T) {
	for in, want := range map[time.Duration]string{
		5 * time.Minute: "5m", 90 * time.Second: "1m30s", time.Hour + time.Second: "1h1s",
		500 * time.Millisecond: "500ms", 0: "0s", 1500 * time.Millisecond: "1.5s",
	} {
		if got := Duration(in).String(); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	ok := map[string]int64{
		"512m": 512 << 20, "1g": 1 << 30, "256MB": 256 << 20, "1Gi": 1 << 30,
		"64k": 64 << 10, "1.5g": 3 << 29, "2GiB": 2 << 30,
	}
	for in, want := range ok {
		got, err := ParseSize(in)
		if err != nil || got.Bytes() != want {
			t.Errorf("%q: got %d, %v; want %d", in, got.Bytes(), err, want)
		}
	}
	for _, in := range []string{"", "512", "lots", "1t", "-1m", "0m"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
	if s := Size(1536 << 20).String(); s != "1536m" {
		t.Errorf("String: %q", s)
	}
}

func TestParseTargetRef(t *testing.T) {
	r, err := ParseTargetRef("api[3]")
	if err != nil || r.Name != "api" || r.Index == nil || *r.Index != 3 || r.String() != "api[3]" {
		t.Errorf("got %+v, %v", r, err)
	}
	r, err = ParseTargetRef("orders-db")
	if err != nil || r.Index != nil || r.String() != "orders-db" {
		t.Errorf("got %+v, %v", r, err)
	}
	for _, in := range []string{"", "API", "api[]", "api[-1]", "api[1", "a b"} {
		if _, err := ParseTargetRef(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}
