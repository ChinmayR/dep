package zapfx

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Configuration toggles options for a zap.Logger. Portions of the open-source
// zap.Config struct are omitted because all internal applications should use
// the ELK team's preferred message schema.
//
// Most services can omit configuration entirely and rely on the
// environment-based defaults.
type Configuration struct {
	// Level sets the minimum enabled log level. Valid values are "debug",
	// "info", "warn", "error", "dpanic", "panic", and "fatal".
	Level string `yaml:"level"`
	// Development enables a handful of behaviors useful during development and
	// testing. First, it alters the behavior of the logger's DPanic ("panic in
	// development") method to panic instead of just writing an error. Second,
	// it reduces console noise by omitting the default hostname, zone, and
	// service name fields.
	Development bool `yaml:"development"`
	// DisableCaller instructs the logger not to log annotations identifying
	// the calling function.
	DisableCaller bool `yaml:"disableCaller"`
	// DisableStacktrace disables zap's automatic stacktrace collection. By
	// default, stacktraces are included in error-and-above logs in production
	// and warn-and-above logs in development.
	DisableStacktrace bool `yaml:"disableStacktrace"`
	// Encoding specifies the output encoding. Valid options are "console" and
	// "json". Note that only JSON-encoded logs will be ingested into Uber's
	// production Kafka and ELK systems.
	Encoding string `yaml:"encoding"`
	// OutputPaths lists the files to send logging output to. The special
	// strings "stdout" and "stderr" are interpreted as standard out and
	// standard error, respectively, and an empty output list no-ops the
	// logger. By default, logs are sent to standard out.
	OutputPaths []string `yaml:"outputPaths"`
	// Sampling configures zap's reservoir sampling. If non-nil, zap logs the
	// first N entries with the same message each second, and every Mth entry
	// thereafter. This prevents a storm of errors from grinding your
	// application to a halt as each logger contends over the output file's
	// mutex.
	Sampling *zap.SamplingConfig `yaml:"sampling"`
	// InitialFields adds predefined context to every log message. To comply
	// with the ELK team's schema, your service's name, the hostname, and the
	// current zone are automatically added to the logger's context in
	// production.
	InitialFields map[string]interface{} `yaml:"initialFields"`
}

func (c Configuration) build() (zap.AtomicLevel, *zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(c.Level)); err != nil {
		return zap.NewAtomicLevel(), nil, fmt.Errorf("couldn't load level %q: %v", c.Level, err)
	}
	atomic := zap.NewAtomicLevelAt(lvl)
	z := zap.Config{
		Level:             atomic,
		Development:       c.Development,
		DisableCaller:     c.DisableCaller,
		DisableStacktrace: c.DisableStacktrace,
		Encoding:          c.Encoding,
		Sampling:          c.Sampling,
		OutputPaths:       c.OutputPaths,
		ErrorOutputPaths:  []string{"stderr"},
		InitialFields:     c.InitialFields,
	}
	if z.Development {
		z.EncoderConfig = zap.NewDevelopmentEncoderConfig()
		logger, err := z.Build()
		return atomic, logger, err
	}
	z.EncoderConfig = zapcore.EncoderConfig{
		// From the Panama logging RFC: t.uber.com/panama-logging
		MessageKey:     "message",
		LevelKey:       "level",
		NameKey:        "logger_name",
		TimeKey:        "ts",
		CallerKey:      "caller",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.EpochTimeEncoder,
		EncodeDuration: zapcore.NanosDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	logger, err := z.Build()
	return atomic, logger, err
}

func (c Configuration) defaultField(key, val string) {
	if _, ok := c.InitialFields[key]; !ok {
		c.InitialFields[key] = val
	}
}
