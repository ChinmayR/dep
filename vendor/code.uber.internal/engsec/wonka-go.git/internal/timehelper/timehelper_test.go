package timehelper

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithinClockSkew(t *testing.T) {
	var testVars = []struct {
		b time.Time
		s time.Duration

		ok bool
	}{
		{time.Now().Add(-time.Second), 2 * time.Second, true},
		{time.Now().Add(time.Second), 2 * time.Second, true},
		{time.Now().Add(-10 * time.Second), 2 * time.Second, false},
		{time.Now().Add(10 * time.Second), 2 * time.Second, false},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("test %d", idx), func(t *testing.T) {
			n := time.Now()
			require.Equal(t, m.ok, WithinClockSkew(n, m.b, m.s))
		})
	}
}
