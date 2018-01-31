package testhelper

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// EnrollEntity creates a database entry for an entity with the given name and
// privatekey.
func EnrollEntity(ctx context.Context, t testing.TB, db wonkadb.EntityDB, name string, privkey *rsa.PrivateKey) {
	entity := wonka.Entity{
		EntityName:   name,
		PublicKey:    string(testdata.PublicPemFromKey(privkey)),
		ECCPublicKey: testdata.ECCPublicFromPrivateKey(privkey),
		Ctime:        int(time.Now().Unix()),
		Etime:        int(time.Now().Add(time.Hour).Unix()),
	}
	err := db.Create(ctx, &entity)
	require.NoError(t, err, "failed to enroll %q", name)
}

// PrivatePemFromKey encodes the given rsa private key into pem format.
func PrivatePemFromKey(k *rsa.PrivateKey) string {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	return string(pem.EncodeToMemory(&pemBlock))
}

// SetupContext returns a mock tracer, context, and mock span suitable for
// testing and inspection.
func SetupContext() (opentracing.Tracer, context.Context, *mocktracer.MockSpan) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("test-span")
	ctx := opentracing.ContextWithSpan(context.Background(), span)
	return tracer, ctx, span.(*mocktracer.MockSpan)
}

// AssertNoSpanFieldsLogged will fail if the given span has been decorated with
// any log fields.
func AssertNoSpanFieldsLogged(t *testing.T, ms *mocktracer.MockSpan) {
	assert.Equal(t, []mocktracer.MockLogRecord{}, ms.Logs(), "unexpected fields logged to span")
}

// ExpectedInboundSpanFields is a shortcut to construct the fields logged to a
// mock inbound span, because it's such a pain to do.
//   []mocktracer.MockLogRecord: [{2017-11-09 14:52:14.253124507 -0800 PST [
//   {galileo.in.has_baggage bool false}
//   {galileo.in.version string galileo-go: 1.3.5-dev}
//   {galileo.in.enforce_percentage float64 0.42}
//   {galileo.in.allowed int 0}
//   /]}]
func ExpectedInboundSpanFields(hasBaggage bool, enforcePercentage float64, allowed int, destination, entityName string) []mocktracer.MockLogRecord {
	expected := []mocktracer.MockLogRecord{
		{Fields: []mocktracer.MockKeyValue{
			{Key: "galileo.in.has_baggage",
				ValueString: strconv.FormatBool(hasBaggage), ValueKind: reflect.Bool},
			{Key: "galileo.in.version",
				ValueString: internal.LibraryVersion(), ValueKind: reflect.String},
			{Key: "galileo.in.enforce_percentage",
				ValueString: fmt.Sprint(enforcePercentage), ValueKind: reflect.Float64},
			{Key: "galileo.in.allowed",
				ValueString: strconv.Itoa(allowed), ValueKind: reflect.Int},
		},
		},
	}

	if destination != "" || entityName != "" {
		additional := []mocktracer.MockKeyValue{
			{Key: "galileo.in.destination", ValueString: destination, ValueKind: reflect.String},
			{Key: "galileo.in.entity_name", ValueString: entityName, ValueKind: reflect.String},
		}
		expected = append(expected, mocktracer.MockLogRecord{Fields: additional})
	}
	return expected
}

// ExpectedOutboundSpanFields is a shortcut to construct the fields logged to a
// mock outbound span, because it's such a pain to do.
func ExpectedOutboundSpanFields(hasBaggage bool, destination, entityName string) []mocktracer.MockLogRecord {
	return []mocktracer.MockLogRecord{
		{Fields: []mocktracer.MockKeyValue{
			{Key: "galileo.out.has_baggage",
				ValueString: strconv.FormatBool(hasBaggage), ValueKind: reflect.Bool},
			{Key: "galileo.out.version",
				ValueString: internal.LibraryVersion(), ValueKind: reflect.String},
			{Key: "galileo.out.destination",
				ValueString: destination, ValueKind: reflect.String},
			{Key: "galileo.out.entity_name",
				ValueString: entityName, ValueKind: reflect.String},
		},
		},
	}
}

// AssertSpanFieldsLogged asserts the given mock span has been decorated with
// the given log records.
func AssertSpanFieldsLogged(t *testing.T, ms *mocktracer.MockSpan, expected []mocktracer.MockLogRecord) {
	require.NotNil(t, ms, "span must not be nil to have logged fields")

	actual := ms.Logs()

	if len(expected) == 0 {
		assert.Equal(t, []mocktracer.MockLogRecord{}, actual, "expected zero fields logged on span")
		return
	}

	zeroOutTimestamps(actual)
	assert.Equal(t, expected, actual, "expected fields are not logged on span")
}

// zeroOutTimestamps clears timestamps from span logs so we can make consistant
// equality checks.
func zeroOutTimestamps(recs []mocktracer.MockLogRecord) {
	for i := range recs {
		recs[i].Timestamp = time.Time{}
	}
}

// AssertNoM3Counters will fail if any counter metrics have been emitted to the
// given tally scope.
func AssertNoM3Counters(t *testing.T, metrics tally.TestScope) {
	counters := metrics.Snapshot().Counters()
	// Using assert.Equal will displays actual value when it isn't empty.
	assert.Equal(t, map[string]tally.CounterSnapshot{}, counters,
		"expected zero M3 counters emitted, but got %d", len(counters))
}

// AssertOneM3Counter asserts there is only one counter and it matches the given
// name, tags, and value.
func AssertOneM3Counter(t *testing.T, metrics tally.TestScope, name string, value int64, tags map[string]string) {
	counters := metrics.Snapshot().Counters()
	assert.Equal(t, 1, len(counters), "expected 1 M3 counter, but got %d", len(counters))
	AssertM3Counter(t, metrics, name, value, tags)
}

// AssertNoM3Counter asserts a counter with the given name has not been emitted.
// Any number of other counter metrics are allowed.
func AssertNoM3Counter(t *testing.T, metrics tally.TestScope, name string) {
	counters := metrics.Snapshot().Counters()
	var counter tally.CounterSnapshot

	// Counters is map[string]CounterSnapshot.
	// The key is a bizarre concatenation of name with tags, so we're
	// searching by Name instead.
	for _, c := range counters {
		if c.Name() == name {
			counter = c
			break
		}
	}

	require.Nil(t, counter, "M3 counter nameed %qshould not have been emitted", name)
}

// AssertM3Counter asserts a counter matching counter has been emitted, amoung
// any number of other counter metrics.
func AssertM3Counter(t *testing.T, metrics tally.TestScope, name string, value int64, tags map[string]string) {
	counters := metrics.Snapshot().Counters()
	var counter tally.CounterSnapshot

	// Counters is map[string]CounterSnapshot.
	// The key is a bizarre concatenation of name with tags, so we're
	// searching by Name instead.
	for _, c := range counters {
		if c.Name() == name {
			counter = c
			break
		}
	}

	require.NotNil(t, counter, "no M3 counter emitted with name %q", name)
	assert.Equal(t, value, counter.Value(), "unexpected value for M3 counter %q", name)
	assert.Equal(t, tags, counter.Tags(), "unexpected tags for M3 counter %q", name)
}

// AssertNoZapLogs asserts no log entries have been added to zap's observable
// logger.
func AssertNoZapLogs(t *testing.T, logs *observer.ObservedLogs) {
	assert.Equal(t, []observer.LoggedEntry{}, logs.All(), "expected zero zap logs, but got %d", logs.Len())
}

// AssertOneZapLog asserts that only one log entry has been added to zap's
// observable logger, and it matches the given parameters.
// Construct your logger and logs object like:
//   obs, logs := observer.New(zap.DebugLevel)
//   logger := zap.New(obs)
// https://godoc.org/go.uber.org/zap/zaptest/observer
func AssertOneZapLog(t *testing.T, logs *observer.ObservedLogs, level zapcore.Level, msg string, fields []zapcore.Field) {
	assert.Equal(t, 1, logs.Len(), "expected 1 zap log, but got %d", logs.Len())

	want := []observer.LoggedEntry{{
		Entry:   zapcore.Entry{Level: level, Message: msg},
		Context: fields,
	}}

	assert.Equal(t, want, logs.AllUntimed(), "unexpected log contents")
}

// AssertZapLog asserts that one matching entry has been added to zap's
// observable logger, amoung any number of other log entries.
// Construct your logger and logs object like:
//   obs, logs := observer.New(zap.DebugLevel)
//   logger := zap.New(obs)
// https://godoc.org/go.uber.org/zap/zaptest/observer
func AssertZapLog(t *testing.T, logs *observer.ObservedLogs, level zapcore.Level, msg string, fields []zapcore.Field) {
	want := []observer.LoggedEntry{{
		Entry:   zapcore.Entry{Level: level, Message: msg},
		Context: fields,
	}}
	matching := logs.FilterMessage(msg).AllUntimed()
	if len(matching) == 0 {
		all := logs.All()
		allActualMessages := make([]string, len(all))
		for i, e := range all {
			allActualMessages[i] = e.Message
		}
		assert.FailNow(t, "expected log entry not found",
			"log message %q is not among log messages %q", msg, allActualMessages)
	}

	assert.Equal(t, want, matching, "fields associated with log entry do not match")
}
