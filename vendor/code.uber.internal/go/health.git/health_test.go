package health

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		in  State
		out string
	}{
		{RefusingTraffic, "refusing"},
		{AcceptingTraffic, "accepting"},
		{Stopping, "stopping"},
		{Stopped, "stopped"},
		{42, "State(42)"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.out, tt.in.String(), "Unexpected string output for state %d.", tt.in)
	}
}

func TestStateUnmarshalText(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		tests := []struct {
			expect State
			text   string
		}{
			{RefusingTraffic, "refusing"},
			{AcceptingTraffic, "accepting"},
			{Stopping, "stopping"},
			{Stopped, "stopped"},
			{AcceptingTraffic, "ACCEPTING"},
			{AcceptingTraffic, "AcCePtInG"},
		}

		for _, tt := range tests {
			var state State
			require.NoError(t, state.UnmarshalText([]byte(tt.text)), "Unexpected error parsing text.")
			assert.Equal(t, tt.expect, state, "Unexpected unmarshaled level.")
		}
	})

	t.Run("Failure", func(t *testing.T) {
		var state State
		err := state.UnmarshalText([]byte("foo"))
		require.Error(t, err, "Expected error.")
		assert.Contains(t, err.Error(), "unknown state", "Unexpected error message.")
	})
}

func TestCoordinatorTransitions(t *testing.T) {
	c := NewCoordinator("foo", CoolDown(time.Hour))
	defer c.cleanup()

	assert.Equal(t, RefusingTraffic, c.State(), "Expected default state to be refusing.")

	assert.NoError(t, c.AcceptTraffic(), "Error accepting.")
	assert.Equal(t, AcceptingTraffic, c.State(), "Expected to be accepting.")
	assert.NoError(t, c.AcceptTraffic(), "Expected re-accepting to be a no-op.")

	assert.NoError(t, c.RefuseTraffic(), "Error refusing.")
	assert.Equal(t, RefusingTraffic, c.State(), "Expected to be refusing.")
	assert.NoError(t, c.RefuseTraffic(), "Expected re-refusing to be a no-op.")

	c.Stop()
	assert.Equal(t, Stopping, c.State(), "Expected to be stopping.")
	assert.Error(t, c.AcceptTraffic(), "Stopping should be irreversible.")
	assert.Error(t, c.RefuseTraffic(), "Stopping should be irreversible.")
}

func TestStoppedSignal(t *testing.T) {
	const concurrency = 100

	c := NewCoordinator("foo", CoolDown(500*time.Millisecond))
	defer c.cleanup()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c.AcceptTraffic()
			c.RefuseTraffic()
			c.AcceptTraffic()
			c.RefuseTraffic()
			c.Stop()
			<-c.Stopped()
		}()
	}

	close(start)
	wg.Wait()
	assert.Equal(t, Stopped, c.State(), "Expected to be stopped.")
}
