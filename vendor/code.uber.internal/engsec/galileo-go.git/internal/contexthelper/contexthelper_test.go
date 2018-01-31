package contexthelper

import (
	"context"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"
	wonka "code.uber.internal/engsec/wonka-go.git"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
)

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
			assert.Equal(t, _galileoSpanName, ms.OperationName, "span should be properly named")
		})
	}
}

func TestEnsureSpanWithSpan(t *testing.T) {
	tracer, ctx, ms := testhelper.SetupContext()

	ensuredCtx, noopFinish := EnsureSpan(ctx, tracer)

	assert.Exactly(t, ctx, ensuredCtx, "context should not change")
	assert.Zero(t, ms.FinishTime, "span should be open")
	noopFinish()
	assert.Zero(t, ms.FinishTime, "finish method should have no effect")
}

func TestAddSpanWithSpan(t *testing.T) {
	tracer, ctx, originalSpan := testhelper.SetupContext()

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

func TestClaimFromContextWithoutSpan(t *testing.T) {
	ctx := context.Background()

	claim, err := ClaimFromContext(ctx)

	assert.Nil(t, claim, "claim should be nil")
	assert.Equal(t, err, internal.ErrNoSpan, "unexpected error")
}

func TestClaimFromContextWithoutClaim(t *testing.T) {
	_, ctx, _ := testhelper.SetupContext()

	claim, err := ClaimFromContext(ctx)

	assert.Nil(t, claim, "claim should be nil")
	assert.Equal(t, err, internal.ErrNoToken, "unexpected error")
}

func TestClaimFromContextMalformedClaim(t *testing.T) {
	_, ctx, ms := testhelper.SetupContext()

	SetBaggage(ms, "malformed value")

	claim, err := ClaimFromContext(ctx)

	assert.Nil(t, claim, "claim should be nil")
	assert.Equal(t, err.Reason(), internal.UnauthorizedMalformedToken, "unexpected reason")
	assert.Contains(t, err.Error(), "unmarshalling claim token", "unexpected error")
	assert.Empty(t, ms.BaggageItem(ServiceAuthBaggageAttr), "baggage should be removed from span")
}

func TestClaimFromContextValidClaim(t *testing.T) {
	testhelper.WithSignedClaim(t, func(inClaim *wonka.Claim, claimString string) {
		_, ctx, ms := testhelper.SetupContext()

		SetBaggage(ms, claimString)

		outClaim, err := ClaimFromContext(ctx)

		assert.NoError(t, err, "ClaimFromContext should succeed")
		assert.Equal(t, inClaim, outClaim, "ClaimFromContext should not modify claim")
		assert.Empty(t, ms.BaggageItem(ServiceAuthBaggageAttr), "baggage should be removed from span")
	})
}
