package logging

import (
	tchannel "github.com/uber/tchannel-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewTChannelLogger builds a new TChannel Logger which logs to the given Zap
// logger.
func NewTChannelLogger(log *zap.Logger, level zap.AtomicLevel) tchannel.Logger {
	return tchannelLogger{
		SugaredLogger: log.Sugar(),
		Level:         level,
	}
}

// tchannelLogger adapts a Zap logger into a tchannel.Logger.
type tchannelLogger struct {
	*zap.SugaredLogger

	// This must point to the logging level used by the corresponding
	// SugaredLogger.
	Level zap.AtomicLevel
}

var _ tchannel.Logger = tchannelLogger{}

func (l tchannelLogger) Enabled(level tchannel.LogLevel) bool {
	var zapLevel zapcore.Level
	switch level {
	case tchannel.LogLevelAll, tchannel.LogLevelDebug:
		zapLevel = zapcore.DebugLevel
	case tchannel.LogLevelInfo:
		zapLevel = zapcore.InfoLevel
	case tchannel.LogLevelWarn:
		zapLevel = zapcore.WarnLevel
	case tchannel.LogLevelError:
		zapLevel = zapcore.ErrorLevel
	case tchannel.LogLevelFatal:
		zapLevel = zapcore.FatalLevel
	default:
		l.SugaredLogger.Warn("Cannot map unknown log level to Zap.",
			zap.Int("level", int(level)),
		)
		return true
	}
	return l.Level.Enabled(zapLevel)
}

// Can't rely on embedded method generation for these because SugaredLogger
// versions accept ...interface{}

func (l tchannelLogger) Fatal(msg string) { l.SugaredLogger.Fatal(msg) }
func (l tchannelLogger) Error(msg string) { l.SugaredLogger.Error(msg) }
func (l tchannelLogger) Warn(msg string)  { l.SugaredLogger.Warn(msg) }
func (l tchannelLogger) Info(msg string)  { l.SugaredLogger.Info(msg) }
func (l tchannelLogger) Debug(msg string) { l.SugaredLogger.Debug(msg) }

func (l tchannelLogger) Fields() tchannel.LogFields {
	l.SugaredLogger.Warn("Fields() call to TChannel Logger is not supported by Zap")
	return nil
}

func (l tchannelLogger) WithFields(fields ...tchannel.LogField) tchannel.Logger {
	zapFields := make([]interface{}, len(fields))
	for i, f := range fields {
		zapFields[i] = zap.Any(f.Key, f.Value)
	}

	// Pass-by-value so we can change in-place.
	l.SugaredLogger = l.SugaredLogger.With(zapFields...)
	return l
}
