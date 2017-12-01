package internal

import (
	"context"
	"reflect"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupContext(t *testing.T, tracer *mocktracer.MockTracer) (context.Context, *mocktracer.MockSpan) {
	span := tracer.StartSpan("test-span")
	ctx := opentracing.ContextWithSpan(context.Background(), span)
	return ctx, span.(*mocktracer.MockSpan)
}

type spanner func(context.Context, opentracing.Tracer) (context.Context, func())

func TestWithoutSpan(t *testing.T) {
	var systemsUnderTest = []struct {
		descr string  // describes the test case
		sut   spanner // function to test
	}{
		{"EnsureSpan", EnsureSpan},
		{"AddSpan", AddSpan},
	}

	for _, m := range systemsUnderTest {
		t.Run(m.descr, func(t *testing.T) {

			ctx := context.Background()
			modifiedCtx, finishSpan := m.sut(ctx, mocktracer.New())

			span := opentracing.SpanFromContext(modifiedCtx)
			assert.NotNil(t, span, "context should now have span")

			ms := span.(*mocktracer.MockSpan)

			assert.Zero(t, ms.FinishTime, "span should be open")
			finishSpan()
			assert.NotZero(t, ms.FinishTime, "finish method should finish span")
			assert.Equal(t, galileoSpanName, ms.OperationName, "span should be properly named")
		})
	}
}

func TestEnsureSpanWithSpan(t *testing.T) {
	tracer := mocktracer.New()
	ctx, ms := setupContext(t, tracer)

	ensuredCtx, noopFinish := EnsureSpan(ctx, tracer)

	assert.Exactly(t, ctx, ensuredCtx, "context should not change")
	assert.Zero(t, ms.FinishTime, "span should be open")
	noopFinish()
	assert.Zero(t, ms.FinishTime, "finish method should have no effect")
}

func TestAddSpanWithSpan(t *testing.T) {
	tracer := mocktracer.New()
	ctx, originalSpan := setupContext(t, tracer)

	modifiedCtx, finish := AddSpan(ctx, tracer)

	ns := opentracing.SpanFromContext(modifiedCtx)
	newSpan := ns.(*mocktracer.MockSpan)

	assert.NotEqual(t, ctx, modifiedCtx, "context should be different")

	assert.Zero(t, newSpan.FinishTime, "new span should be open")
	assert.Zero(t, originalSpan.FinishTime, "original span should be open")

	finish()

	assert.NotZero(t, newSpan.FinishTime, "finish method should finish new span")
	assert.Zero(t, originalSpan.FinishTime, "original span should still be open")
}

func TestSetBaggageWithoutSpan(t *testing.T) {
	ctx := context.Background()
	err := SetBaggage(ctx, "wonkaSample:test", "other-service", "good value")
	assert.EqualError(t, err, errorSpanlessSet.Error(), "expected error")
}

func TestSetBaggage(t *testing.T) {
	var attrTestVars = []struct {
		desc string // description of the test case

		name        string // caller
		destination string // callee
		value       string // stand in for the claim itself

		shouldErr bool
	}{
		{desc: "with existing span", name: "wonkaSample:test", destination: "other-service", value: "good value"},
		{desc: "claim with comma", name: "wonkaSample:naughty", destination: "other-service", value: "comma,value", shouldErr: false},
	}

	for _, m := range attrTestVars {
		t.Run(
			m.desc,
			func(t *testing.T) {
				tracer := mocktracer.New()
				ctx, ms := setupContext(t, tracer)

				err := SetBaggage(ctx, m.name, m.destination, m.value)
				if m.shouldErr {
					require.EqualError(t, err, errorSpanlessSet.Error(), "expected error")
					return
				}
				assert.NoError(t, err, "expected success")

				assert.Equal(t, m.value, ms.BaggageItem(serviceAuthBaggageAttr), "claim value should be set in baggage")

				assertFieldsLogged(t, ms, []mocktracer.MockKeyValue{
					{Key: tagOutHasBaggage, ValueKind: reflect.Bool, ValueString: "true"},
					{Key: tagOutDestination, ValueKind: reflect.String, ValueString: m.destination},
					{Key: tagOutEntityName, ValueKind: reflect.String, ValueString: m.name},
					{Key: tagOutVersion, ValueKind: reflect.String, ValueString: libraryVersion()},
				})
			})
	}
}

func TestClaimFromContextWithoutSpan(t *testing.T) {
	ctx := context.Background()
	result, err := ClaimFromContext(ctx)
	assert.EqualError(t, err, errorSpanlessGet.Error(), "expected error")
	assert.Empty(t, result, "result should be empty")
}

func TestClaimFromContextWithoutClaim(t *testing.T) {
	tracer := mocktracer.New()
	ctx, ms := setupContext(t, tracer)

	result, err := ClaimFromContext(ctx)
	assert.Empty(t, result, "result should be empty")
	assert.NoError(t, err, "should successfully retrieve zero claims")

	assertFieldsLogged(t, ms, []mocktracer.MockKeyValue{
		{Key: tagInHasBaggage, ValueKind: reflect.Bool, ValueString: "false"},
		{Key: tagInVersion, ValueKind: reflect.String, ValueString: libraryVersion()},
	})
}

func TestClaimFromContext(t *testing.T) {
	var attrTestVars = []struct {
		desc string // description of the test case

		value string // stand in for the claim itself

		shouldErr bool
	}{
		{desc: "with existing span", value: "good value"},
	}

	for _, m := range attrTestVars {
		t.Run(
			m.desc,
			func(t *testing.T) {
				tracer := mocktracer.New()
				ctx, ms := setupContext(t, tracer)

				ms.SetBaggageItem(serviceAuthBaggageAttr, m.value)

				result, err := ClaimFromContext(ctx)
				assert.NoError(t, err, "should successfully retrieve claim")

				assert.Equal(t, m.value, result, "claim value should be returned")

				assertFieldsLogged(t, ms, []mocktracer.MockKeyValue{
					{Key: tagInHasBaggage, ValueKind: reflect.Bool, ValueString: "true"},
					{Key: tagInVersion, ValueKind: reflect.String, ValueString: libraryVersion()},
				})
			})
	}
}

func assertFieldsLogged(t *testing.T, ms *mocktracer.MockSpan, fields []mocktracer.MockKeyValue) {
	actual := ms.Logs()
	zeroOutTimestamps(actual)
	expected := []mocktracer.MockLogRecord{
		{Fields: fields},
	}
	assert.Equal(t, expected, actual, "expected fields are not logged on span")
}

func zeroOutTimestamps(recs []mocktracer.MockLogRecord) {
	for i := range recs {
		recs[i].Timestamp = time.Time{}
	}
}
