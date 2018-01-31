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
	l.Info("Creating new handler for /tally...")
	mux := http.NewServeMux()
	h := http.HandlerFunc(record(s, l))
	mux.Handle("/tally", h)
	mux.Handle("/tally/", h)
	mux.Handle("/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	l.Info("/tally handler created successfully")
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
		l.Info(r.Method + " request received from " + r.RemoteAddr)
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

		l.Info("request form parsed successfully, determining type of metric...")

		for k, v := range r.Form {
			switch k {
			case types.CounterKey:
				var c types.Counter
				l.Info(fmt.Sprintf("%d counters received from %s", len(v), r.RemoteAddr))
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
					l.Info(fmt.Sprintf("Counter '%s' with count of %d logged successfully", c.Name, c.Value))
				}

			case types.GaugeKey:
				var g types.Gauge
				l.Info(fmt.Sprintf("%d gauges received from %s", len(v), r.RemoteAddr))
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
					l.Info(fmt.Sprintf("Gauge '%s' with value of %f logged successfully", g.Name, g.Value))
				}

			case types.TimerKey:
				var t types.Timer
				l.Info(fmt.Sprintf("%d timers received from %s", len(v), r.RemoteAddr))
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
					l.Info(fmt.Sprintf("Timer '%s' with value of %d logged successfully", t.Name, t.Interval))
				}

			case types.HValueKey:
				var h types.HistogramValue
				l.Info(fmt.Sprintf("%d histograms received from %s", len(v), r.RemoteAddr))
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
					l.Info(fmt.Sprintf("Histogram '%s' logged successfully", h.Name))
				}

			case types.HDurationKey:
				var h types.HistogramDuration
				l.Info(fmt.Sprintf("%d histogram durations received from %s", len(v), r.RemoteAddr))
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
					l.Info(fmt.Sprintf("Histogram duration '%s' logged successfully", h.Name))
				}

			default:
				l.Error("Unable to determine metric type from " + r.RemoteAddr)
			}
		}
	}
}
