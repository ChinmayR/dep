package wonka

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// IsGloballyDisabled returns true if wonka is disabled and false otherwise.
func IsGloballyDisabled(w Wonka) bool {
	uw, ok := w.(*uberWonka)
	if !ok {
		return false
	}

	return uw.IsGloballyDisabled()
}

// IsGloballyDisabled returns true if Wonka is marked as being globally disabled.
//
// The disabled state is stored locally and returned by this function. It is refreshed
// in a background routine if is detected as being stale beyond a certain threshold.
func (w *uberWonka) IsGloballyDisabled() bool {
	return w.globalDisableReader.IsDisabled()
}

func (w *uberWonka) performGlobalDisableRecovery(ctx context.Context) {
	// Try in a loop until we succeed
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// We need to make sure that wonkaURL is set
			err := w.httpRequester.SetURL(ctx, w.wonkaURLRequested)
			if err != nil {
				w.log.Error("Failed to set wonka URL after recovery from global disable", zap.Error(err))
				continue
			}
			err = w.upgradeCGCert(ctx)
			if err != nil {
				w.log.Error("Failed to upgrade to certificate after recovery from global disable", zap.Error(err))
				continue
			}

			w.log.Info("Recovered from global disable state")
			return
		case <-ctx.Done():
			w.log.Info("Aborting global disable recovery due to context cancellation")
			return
		}
	}
}
