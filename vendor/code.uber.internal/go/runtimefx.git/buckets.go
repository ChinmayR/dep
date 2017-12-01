package runtimefx

import (
	"sort"

	"github.com/uber-go/tally"
)

func combineDurations(buckets ...tally.DurationBuckets) tally.DurationBuckets {
	res := tally.DurationBuckets{}
	for _, b := range buckets {
		res = append(res, b...)
	}
	sort.Sort(res)
	return res
}

func combineValues(buckets ...tally.ValueBuckets) tally.ValueBuckets {
	res := tally.ValueBuckets{}
	for _, b := range buckets {
		res = append(res, b...)
	}
	sort.Sort(res)
	return res
}
