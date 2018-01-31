package wonka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// IsDerelict returns true if this entity name is a derelict and is allowed to
// bypass any kind of secure authentication. The owners of these services
// should feel bad about their membership on this list because they are
// negatively impacting the security of every other service.
func IsDerelict(w Wonka, entity string) bool {
	uw, ok := w.(*uberWonka)
	if !ok {
		return false
	}

	return uw.IsDerelict(entity)
}

func (w *uberWonka) IsDerelict(entity string) bool {
	if entity == "" {
		return false
	}

	select {
	case e := <-w.derelictsTimer.C:
		w.log.With(zap.Time("expired", e), zap.Time("now", time.Now())).Debug("Refreshing derelicts")
		go w.refreshDerelicts(context.Background())
	default:
	}

	w.derelictsLock.RLock()
	expTime, ok := w.derelicts[entity]
	w.derelictsLock.RUnlock()

	if !ok {
		return false
	}

	return time.Now().Before(expTime)
}

// refreshDerelicts updates the derelicts data and resets the derelict timer
// so it can fire again. This function is expected to only be called in response
// to the timer firing.
func (w *uberWonka) refreshDerelicts(ctx context.Context) {
	defer func() {
		// Regardless of whether updating derelicts succeeded or not, we need to
		// reset the timer so we can try refreshing again later. If the call succeeded
		// we'll use the new derelictsRefreshPeriod, otherwise we'll use what was already
		// set, possibly the default.
		w.derelictsLock.Lock()
		w.derelictsTimer.Reset(w.derelictsRefreshPeriod)
		w.derelictsLock.Unlock()
	}()

	if err := w.updateDerelicts(ctx); err != nil {
		w.log.With(zap.Error(err)).Warn("failed to update derelicts")
	}
}

func (w *uberWonka) updateDerelicts(ctx context.Context) error {
	if w.IsGloballyDisabled() {
		return errors.New("globally disabled, skipping derelict update")
	}

	req := TheHoseRequest{
		EntityName: w.entityName,
		Ctime:      int64(time.Now().Unix()),
	}

	toSign, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshalling request: %v", err)
	}

	req.Signature, err = w.Sign(toSign)
	if err != nil {
		return fmt.Errorf("error signing request: %v", err)
	}

	var reply TheHoseReply
	if err := w.httpRequester.Do(ctx, hoseEndpoint, req, &reply); err != nil {
		w.metrics.Tagged(map[string]string{
			"error": err.Error(),
		}).Counter("check_derelicts").Inc(1)
		return fmt.Errorf("error making https request: %v", err)
	}

	// TODO(pmoody): check the heckin' signature on the reply
	for k, v := range reply.Derelicts {
		w.log.Debug("adding derelict service",
			zap.String("service", k), zap.Time("expireTime", v))
	}

	w.derelictsLock.Lock()
	w.derelicts = reply.Derelicts
	w.derelictsRefreshPeriod = time.Duration(reply.CheckInterval) * time.Second
	w.derelictsLock.Unlock()

	return nil
}
