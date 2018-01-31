package logging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	tchannel "github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"code.uber.internal/engsec/wonka-go.git/internal/logging"
)

func TestEnabled(t *testing.T) {
	l := logging.NewTChannelLogger(zap.NewNop(), zap.NewAtomicLevel())
	assert.False(t, l.Enabled(tchannel.LogLevelDebug), "zap level reported unexpected result")
	assert.True(t, l.Enabled(tchannel.LogLevelInfo), "zap level reported unexpected result")
	assert.True(t, l.Enabled(tchannel.LogLevelWarn), "zap level reported unexpected result")
	assert.True(t, l.Enabled(tchannel.LogLevelError), "zap level reported unexpected result")
	assert.True(t, l.Enabled(tchannel.LogLevelFatal), "zap level reported unexpected result")
	assert.True(t, l.Enabled(tchannel.LogLevel(202)), "zap level reported unexpected result")
}

func TestFields(t *testing.T) {
	l := logging.NewTChannelLogger(zap.NewNop(), zap.NewAtomicLevel())
	assert.Nil(t, l.Fields(), "expected nil but it was not")
}

func TestWithFields(t *testing.T) {
	fields := tchannel.LogField{
		Key:   "key",
		Value: "field",
	}
	l := logging.NewTChannelLogger(zap.NewNop(), zap.NewAtomicLevel())
	assert.Equal(t, l, l.WithFields(fields))
}
