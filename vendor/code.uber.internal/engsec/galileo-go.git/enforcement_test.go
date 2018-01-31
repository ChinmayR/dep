package galileo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code.uber.internal/engsec/galileo-go.git/internal/atomic"
)

func TestEnforcePercentage(t *testing.T) {
	tests := []struct {
		updatedPercentage float64
		errString         string
	}{
		{updatedPercentage: 0},
		{updatedPercentage: 0.5},
		{updatedPercentage: 1},
		{updatedPercentage: 2, errString: "enforce percentage should be from [0.0, 1.0]"},
		{updatedPercentage: 10000, errString: "enforce percentage should be from [0.0, 1.0]"},
		{updatedPercentage: -10000, errString: "enforce percentage should be from [0.0, 1.0]"},
	}
	g := &uberGalileo{
		enforcePercentage: atomic.NewFloat64(0),
	}
	for _, tt := range tests {
		err := SetConfig(g, EnforcePercentage(tt.updatedPercentage))
		if tt.errString != "" {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errString)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tt.updatedPercentage, g.enforcePercentage.Load())
		}
	}
}
