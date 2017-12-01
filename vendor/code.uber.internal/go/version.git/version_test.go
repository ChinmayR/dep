package version

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetBuildTime(t *testing.T) {
	tests := []struct {
		seconds string
		want    time.Time
	}{
		{
			// successful
			seconds: "1457074206",
			want:    time.Unix(1457074206, 0),
		},
		{
			// default time is used when value can't be parsed
			seconds: "unknown",
		},
	}

	var zeroTime time.Time
	for _, tt := range tests {
		BuildTime = zeroTime

		buildUnixSeconds = tt.seconds
		setBuildTime()
		assert.Equal(t, tt.want, BuildTime, "BuildTime not set correctly")
	}

}
