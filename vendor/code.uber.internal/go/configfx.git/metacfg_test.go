package configfx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedupeSlice(t *testing.T) {
	tests := []struct {
		in, exp []string
	}{
		{in: []string{"a"}, exp: []string{"a"}},
		{in: []string{"a", "a"}, exp: []string{"a"}},
		{in: []string{"a", "b", "a"}, exp: []string{"b", "a"}},
		{in: []string{"a", "b", "a", "b"}, exp: []string{"a", "b"}},
	}

	for _, test := range tests {
		out := dedupeStringSlice(test.in)
		assert.Equal(t, test.exp, out)
	}
}
