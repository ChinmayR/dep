package galileo

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAllowedEntities(t *testing.T) {
	tests := []struct {
		desc         string
		give         error
		wantEntities []string
		wantMessage  string
	}{
		{
			desc:         "not an authError",
			give:         errors.New("great sadness"),
			wantEntities: []string{"EVERYONE"},
			wantMessage:  "great sadness",
		},
		{
			desc: "authError without entities",
			give: &authError{
				Reason: errors.New("great sadness"),
			},
			wantEntities: []string{"EVERYONE"},
			wantMessage:  "great sadness",
		},
		{
			desc: "authError with entities",
			give: &authError{
				Reason:          errors.New("great sadness"),
				AllowedEntities: []string{"foo", "bar"},
			},
			wantEntities: []string{"foo", "bar"},
			wantMessage:  "great sadness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			gotEntities := GetAllowedEntities(tt.give)
			assert.Equal(t, tt.wantEntities, gotEntities)
			assert.Equal(t, tt.wantMessage, tt.give.Error())
		})
	}
}
