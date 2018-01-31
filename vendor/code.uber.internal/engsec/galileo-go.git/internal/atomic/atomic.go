// Package atomic contains selectively vendored routines from go.uber.org/atomic v1.2.0
package atomic

import (
	"math"
	"sync/atomic"
)

// Float64 is an atomic wrapper around float64.
type Float64 struct {
	v uint64
}

// NewFloat64 creates a Float64.
func NewFloat64(f float64) *Float64 {
	return &Float64{math.Float64bits(f)}
}

// Load atomically loads the wrapped value.
func (f *Float64) Load() float64 {
	return math.Float64frombits(atomic.LoadUint64(&f.v))
}

// Store atomically stores the passed value.
func (f *Float64) Store(s float64) {
	atomic.StoreUint64(&f.v, math.Float64bits(s))
}
