package wonka

import (
	"context"
	"encoding/json"
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

	if entity == "" {
		return false
	}

	uw.derelictsLock.RLock()
	expTime, ok := uw.derelicts[entity]
	uw.derelictsLock.RUnlock()

	if !ok {
		return false
	}

	return time.Now().Before(expTime)
}

func (w *uberWonka) checkDerelicts(ctx context.Context, period time.Duration) {
	timer := time.NewTimer(period)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if p := w.updateDerelicts(ctx); p > 0 {
				period = p
			}
			timer.Reset(period)
		}
	}
}

func (w *uberWonka) updateDerelicts(ctx context.Context) time.Duration {
	req := TheHoseRequest{
		EntityName: w.entityName,
		Ctime:      int64(time.Now().Unix()),
	}

	toSign, err := json.Marshal(req)
	if err != nil {
		w.log.Warn("error marshaling request", zap.Error(err))
		return 0
	}

	req.Signature, err = w.Sign(toSign)
	if err != nil {
		w.log.Warn("error signing request", zap.Error(err))
		return 0
	}

	var reply TheHoseReply
	if err := w.httpRequest(ctx, hoseEndpoint, req, &reply); err != nil {
		w.metrics.Tagged(map[string]string{
			"error": err.Error(),
		}).Counter("check_derelicts").Inc(1)
		return time.Duration(0)
	}

	// TODO(pmoody): check the heckin' signature on the reply
	for k, v := range reply.Derelicts {
		w.log.Debug("adding derelict service",
			zap.String("service", k), zap.Time("expireTime", v))
	}

	w.derelictsLock.Lock()
	w.derelicts = reply.Derelicts
	w.derelictsLock.Unlock()

	return time.Duration(reply.CheckInterval) * time.Second
}
