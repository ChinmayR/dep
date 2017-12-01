package sentryfx

import (
	"sync"
	"testing"
	"time"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"

	raven "github.com/getsentry/raven-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type spy struct {
	sync.Mutex
	packets []*raven.Packet
	waits   int
}

func (s *spy) Capture(p *raven.Packet, tags map[string]string) (string, chan error) {
	if len(tags) > 0 {
		// Sentry indexes tags for search, which is expensive. Ensure that we're
		// not trying to index arbitrary logger context.
		panic("Sentry integration shouldn't use on capture-site tags.")
	}
	s.Lock()
	defer s.Unlock()
	s.packets = append(s.packets, p)
	return "", nil
}

func (s *spy) Wait() {
	s.Lock()
	s.waits++
	s.Unlock()
}

func (s *spy) Packets() []*raven.Packet {
	s.Lock()
	defer s.Unlock()
	if len(s.packets) == 0 {
		return nil
	}
	return append([]*raven.Packet{}, s.packets...)
}

func TestSeverity(t *testing.T) {
	tests := []struct {
		z zapcore.Level
		r raven.Severity
	}{
		{zap.DebugLevel, raven.INFO},
		{zap.InfoLevel, raven.INFO},
		{zap.WarnLevel, raven.WARNING},
		{zap.ErrorLevel, raven.ERROR},
		{zap.DPanicLevel, raven.FATAL},
		{zap.PanicLevel, raven.FATAL},
		{zap.FatalLevel, raven.FATAL},
		{zapcore.Level(-42), raven.FATAL},
		{zapcore.Level(100), raven.FATAL},
	}

	for _, tt := range tests {
		assert.Equal(
			t,
			tt.r,
			ravenSeverity(tt.z),
			"Unexpected output converting zap Level %s to raven Severity.", tt.z,
		)
	}
}

func TestCoreFieldOrdering(t *testing.T) {
	// Later fields should overwrite earlier ones if they have the same key.
	core := &core{}
	fields := core.
		with([]zapcore.Field{zap.String("foo", "one"), zap.String("bar", "one")}).
		with([]zapcore.Field{zap.String("foo", "two"), zap.String("bar", "two")}).
		with([]zapcore.Field{zap.String("foo", "three"), zap.String("bar", "three")}).
		extra()
	assert.Equal(
		t,
		map[string]interface{}{"foo": "three", "bar": "three"},
		fields,
		"Unexpected extra context with repeated field keys.",
	)
}

func TestCoreWith(t *testing.T) {
	root := &core{LevelEnabler: zap.ErrorLevel}
	assert.Equal(t, 0, len(root.extra()), "Unexpected context on root logger.")

	// Ensure that we're not sharing context between siblings.
	parent := root.With([]zapcore.Field{zap.String("parent", "parent")}).(*core)
	elder := parent.With([]zapcore.Field{zap.String("elder", "elder")}).(*core)
	younger := parent.With([]zapcore.Field{zap.String("younger", "younger")}).(*core)

	assert.Equal(t, map[string]interface{}{
		"parent": "parent",
	}, parent.extra(), "Unexpected fields on parent.")
	assert.Equal(t, map[string]interface{}{
		"parent": "parent",
		"elder":  "elder",
	}, elder.extra(), "Unexpected fields on first child core.")
	assert.Equal(t, map[string]interface{}{
		"parent":  "parent",
		"younger": "younger",
	}, younger.extra(), "Unexpected fields on second child core.")
}

func TestCoreCheck(t *testing.T) {
	core := &core{LevelEnabler: zap.ErrorLevel}
	assert.Nil(t, core.Check(zapcore.Entry{}, nil), "Expected nil CheckedEntry for disabled levels.")
	ent := zapcore.Entry{Level: zapcore.ErrorLevel}
	assert.NotNil(t, core.Check(ent, nil), "Expected non-nil CheckedEntry for enabled levels.")
}

func TestConfigWrite(t *testing.T) {
	sentry := &spy{}
	core := &core{
		LevelEnabler: zap.ErrorLevel,
		client:       sentry,
	}

	// Write a panic-level message, which should also fire a Sentry event.
	ent := zapcore.Entry{Message: "oh no", Level: zapcore.PanicLevel, Time: time.Now()}
	ce := core.With([]zapcore.Field{zap.String("foo", "bar")}).Check(ent, nil)
	require.NotNil(t, ce, "Expected Check to return non-nil CheckedEntry at enabled levels.")
	ce.Write(zap.String("bar", "baz"))

	// Assert that we wrote and flushed a packet.
	require.Equal(t, 1, len(sentry.packets), "Expected to write one Sentry packet.")
	assert.Equal(t, 1, sentry.waits, "Expected to flush buffered events before crashing.")

	// Assert that the captured packet is shaped correctly.
	p := sentry.packets[0]
	assert.Equal(t, "oh no", p.Message, "Unexpected message in captured packet.")
	assert.Equal(t, raven.FATAL, p.Level, "Unexpected severity in captured packet.")
	require.Equal(t, 1, len(p.Interfaces), "Expected a stacktrace in packet interfaces.")
	trace, ok := p.Interfaces[0].(*raven.Stacktrace)
	require.True(t, ok, "Expected only interface in packet to be a stacktrace.")
	// Trace should contain this test and testing harness main.
	require.Equal(t, 2, len(trace.Frames), "Expected stacktrace to contain at least two frames.")

	frame := trace.Frames[len(trace.Frames)-1]
	assert.Equal(t, "TestConfigWrite", frame.Function, "Expected frame to point to this test function.")
}

func TestModuleSuccess(t *testing.T) {
	env := envfx.Context{
		Deployment:  "prod01",
		Environment: "production",
	}
	sfx := servicefx.Metadata{}

	t.Run("disabled", func(t *testing.T) {
		ver := &versionfx.Reporter{}
		lc := fxtest.NewLifecycle(t)
		result, err := New(Params{
			Service:     sfx,
			Environment: env,
			Config:      config.NopProvider{},
			Lifecycle:   lc,
			Reporter:    ver,
		})
		require.NoError(t, err, "Unexpected error with Sentry disabled.")
		assert.Equal(t, zapcore.NewNopCore(), result.Core, "Expected no-op core with Sentry disabled")
		assert.Equal(t, Version, ver.Version(_name), "Wrong package version reported.")
		lc.RequireStart().RequireStop()
	})

	t.Run("enabled", func(t *testing.T) {
		ver := &versionfx.Reporter{}
		lc := fxtest.NewLifecycle(t)
		cfg, err := config.NewStaticProvider(map[string]interface{}{ConfigurationKey: map[string]interface{}{
			"dsn":           "http://user:pass@sentry.local.uber.internal/123",
			"inAppPrefixes": []string{"one", "two"},
		}})
		require.NoError(t, err, "could not initialize new static provider")
		result, err := New(Params{
			Service:     sfx,
			Environment: env,
			Config:      cfg,
			Lifecycle:   lc,
			Reporter:    ver,
		})
		require.NoError(t, err, "Unexpected error with Sentry disabled.")
		require.NotEqual(t, zapcore.NewNopCore(), result.Core, "Got no-op core with Sentry enabled.")
		assert.Equal(t, Version, ver.Version(_name), "Wrong package version reported.")
		lc.RequireStart().RequireStop()
	})
}

func TestModuleInvalidConfig(t *testing.T) {
	env := envfx.Context{}
	sfx := servicefx.Metadata{}

	assertFails := func(t testing.TB, msg string, cfg map[string]interface{}) {
		lc := fxtest.NewLifecycle(t)
		p, err := config.NewStaticProvider(cfg)
		require.NoError(t, err, "could not initialize new static provider")
		_, err = New(Params{
			Service:     sfx,
			Environment: env,
			Config:      p,
			Lifecycle:   lc,
			Reporter:    &versionfx.Reporter{},
		})
		require.Error(t, err, "Expected an error with invalid configuration.")
		assert.Contains(t, err.Error(), msg, "Unexpected error message.")
		lc.RequireStart().RequireStop()
	}

	t.Run("DSN", func(t *testing.T) {
		assertFails(t, "failed to create Sentry", map[string]interface{}{ConfigurationKey: map[string]interface{}{
			"level":         "error",
			"dsn":           "foobarbaz",
			"inAppPrefixes": []string{"one", "two"},
		}})
	})

	t.Run("DSN", func(t *testing.T) {
		assertFails(t, "isn't a recognized zap logging level", map[string]interface{}{
			ConfigurationKey: map[string]interface{}{
				"level":         "foobar",
				"dsn":           "http://user:pass@sentry.local.uber.internal/123",
				"inAppPrefixes": []string{"one", "two"},
			},
		})
	})
}

func TestVersionReportError(t *testing.T) {
	ver := &versionfx.Reporter{}
	ver.Report(_name, Version)
	params := Params{
		Reporter: ver,
	}
	_, err := New(params)
	assert.Contains(t, err.Error(), "already registered version")
}
