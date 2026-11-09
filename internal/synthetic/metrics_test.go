package synthetic

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// metricValue returns a counter or gauge value, or a histogram's sample
// count, for the series with the given labels (a subset match).
func metricValue(t *testing.T, s *Service, name string, labels map[string]string) float64 {
	t.Helper()
	fams, err := s.m.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
	next:
		for _, m := range f.GetMetric() {
			for k, v := range labels {
				if labelValue(m, k) != v {
					continue next
				}
			}
			switch {
			case m.Counter != nil:
				return m.GetCounter().GetValue()
			case m.Gauge != nil:
				return m.GetGauge().GetValue()
			case m.Histogram != nil:
				return float64(m.GetHistogram().GetSampleCount())
			}
		}
	}
	return 0
}

// histogramSum returns a histogram's sum for the series with the labels.
func histogramSum(t *testing.T, s *Service, name string, labels map[string]string) float64 {
	t.Helper()
	fams, err := s.m.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
	next:
		for _, m := range f.GetMetric() {
			for k, v := range labels {
				if labelValue(m, k) != v {
					continue next
				}
			}
			return m.GetHistogram().GetSampleSum()
		}
	}
	return 0
}

func labelValue(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}
