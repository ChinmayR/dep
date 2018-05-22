package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"code.uber.internal/devexp/proxy-reporter.git/types"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const _errorTag = "error"

// New returns a new http.Handler that can be used for reporting metrics.
func New(s tally.Scope, l *zap.Logger) (http.Handler, error) {
	mux := http.NewServeMux()
	h := http.HandlerFunc(record(s, l))
	mux.Handle("/tally", h)
	mux.Handle("/tally/", h)
	mux.Handle("/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	return mux, nil
}

func toolTag(i *types.Identity, tool string) {
	if i.Tags == nil {
		i.Tags = make(map[string]string)
	}

	i.Tags["tool"] = tool
}

func record(s tally.Scope, l *zap.Logger) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		tool, err := r.Cookie("tool")
		if err != nil {
			http.Error(w, "no tool is specified", http.StatusBadRequest)
			l.Error("no tool specified", zap.Error(err))
			s.Tagged(map[string]string{"path": "cookie"}).Counter(_errorTag).Inc(1)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprintf("error parsing form: %v", err), http.StatusBadRequest)
			l.Error("parsing request form", zap.Error(err))
			s.Tagged(map[string]string{"tool": tool.Value, "path": "form parse"}).Counter(_errorTag).Inc(1)
			return
		}

		for k, v := range r.Form {
			switch k {
			case types.CounterKey:
				var c types.Counter
				for _, i := range v {
					if err := json.Unmarshal([]byte(i), &c); err != nil {
						l.Error("decoding counter", zap.Error(err))
						s.Tagged(map[string]string{
							"tool": tool.Value,
							"path": "counter parse"},
						).Counter(_errorTag).Inc(1)

						continue
					}

					toolTag(&c.Identity, tool.Value)
					s.Tagged(c.Tags).Counter(c.Name).Inc(c.Value)
				}

			case types.GaugeKey:
				var g types.Gauge
				for _, i := range v {
					if err := json.Unmarshal([]byte(i), &g); err != nil {
						l.Error("decoding gauge", zap.Error(err))
						s.Tagged(map[string]string{
							"tool": tool.Value,
							"path": "gauge parse"},
						).Counter(_errorTag).Inc(1)

						continue
					}

					toolTag(&g.Identity, tool.Value)
					s.Tagged(g.Tags).Gauge(g.Name).Update(g.Value)
				}

			case types.TimerKey:
				var t types.Timer
				for _, i := range v {
					if err := json.Unmarshal([]byte(i), &t); err != nil {
						l.Error("decoding timer", zap.Error(err))
						s.Tagged(map[string]string{
							"tool": tool.Value,
							"path": "timer parse"},
						).Counter(_errorTag).Inc(1)

						continue
					}

					toolTag(&t.Identity, tool.Value)
					s.Tagged(t.Tags).Timer(t.Name).Record(t.Interval)
				}

			case types.HValueKey:
				var h types.HistogramValue
				for _, i := range v {
					if err := json.Unmarshal([]byte(i), &h); err != nil {
						l.Error("decoding histogram value", zap.Error(err))
						s.Tagged(map[string]string{
							"tool": tool.Value,
							"path": "histogram value parse"},
						).Counter(_errorTag).Inc(1)

						continue
					}

					toolTag(&h.Identity, tool.Value)
					hist := s.Tagged(h.Tags).Histogram(h.Name, tally.ValueBuckets(h.Values))
					for i := int64(0); i < h.Samples; i++ {
						hist.RecordValue(h.UpperBound)
					}
				}

			case types.HDurationKey:
				var h types.HistogramDuration
				for _, i := range v {
					if err := json.Unmarshal([]byte(i), &h); err != nil {
						l.Error("decoding histogram duration", zap.Error(err))
						s.Tagged(map[string]string{
							"tool": tool.Value,
							"path": "histogram duration parse"},
						).Counter(_errorTag).Inc(1)

						continue
					}

					toolTag(&h.Identity, tool.Value)
					hist := s.Tagged(h.Tags).Histogram(h.Name, tally.DurationBuckets(h.Values))
					for i := int64(0); i < h.Samples; i++ {
						hist.RecordDuration(h.UpperBound)
					}
				}
			}
		}
	}
}
