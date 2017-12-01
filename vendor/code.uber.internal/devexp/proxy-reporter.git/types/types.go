package types

import (
	"time"
)

// Identity defines a common fields to all metrics: name and tags.
type Identity struct {
	Name string
	Tags map[string]string
}

// Counter represents reporting of tally.Counter.
type Counter struct {
	Identity
	Value int64
}

// Gauge represents reporting of tally.Gauge.
type Gauge struct {
	Identity
	Value float64
}

// Timer represents reporting of tally.Timer.
type Timer struct {
	Identity
	Interval time.Duration
}

// HistogramValue represents reporting of tally.HistogramValue.
type HistogramValue struct {
	Identity
	Values     []float64
	LowerBound float64
	UpperBound float64
	Samples    int64
}

// HistogramDuration represents reporting of tally.HistogramDuration.
type HistogramDuration struct {
	Identity
	Values     []time.Duration
	LowerBound time.Duration
	UpperBound time.Duration
	Samples    int64
}

// Form keys for different metric types.
const (
	CounterKey   = "counter"
	GaugeKey     = "gauge"
	TimerKey     = "timer"
	HValueKey    = "histogram_value"
	HDurationKey = "histogram_duration"
	Version      = "0.1.3"
)
