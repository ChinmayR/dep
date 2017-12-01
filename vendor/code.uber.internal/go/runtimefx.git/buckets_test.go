package runtimefx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
)

func TestBucketsAreSorted(t *testing.T) {
	vb := combineValues(
		tally.MustMakeLinearValueBuckets(100, 100, 2),
		tally.MustMakeLinearValueBuckets(0, 10, 2),
	)
	assert.Equal(t, tally.ValueBuckets{0, 10, 100, 200}, vb)

	db := combineDurations(
		tally.MustMakeLinearDurationBuckets(100, 100, 2),
		tally.MustMakeLinearDurationBuckets(0, 10, 2),
	)
	assert.Equal(t, tally.DurationBuckets{0, 10, 100, 200}, db)
}
