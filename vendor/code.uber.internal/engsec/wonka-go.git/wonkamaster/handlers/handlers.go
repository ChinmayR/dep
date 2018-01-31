package handlers

import (
	"go.uber.org/zap"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
)

// SetupHandlers sets up the handlers.
func SetupHandlers(r common.Router, cfg common.HandlerConfig) {
	var err error
	if cfg.UsshHostSigner == nil {
		cfg.UsshHostSigner, err = wonka.ParseHostCA("")
		if err != nil {
			cfg.Logger.Error("error parsing host signer", zap.Error(err))
		}
	}

	r.AddPatternRoute("/admin", newAdminHandler(cfg))
	r.AddPatternRoute("/claim/v2", newClaimHandler(cfg))
	r.AddPatternRoute("/csr", newCSRHandler(cfg))
	r.AddPatternRoute("/destroy", newDestroyHandler(cfg))
	r.AddPatternRoute("/enroll", newEnrollHandler(cfg))
	r.AddPatternRoute("/health", newHealthHandler(cfg))
	r.AddPatternRoute("/lookup", newLookupHandler(cfg))
	r.AddPatternRoute("/resolve", newResolveHandler(cfg))
	// /thehose is going to go away
	r.AddPatternRoute("/thehose", newHoseHandler(cfg))
	r.AddPatternRoute("/hose", newHoseHandler(cfg))

	// this has to go last, or everything dies
	r.AddPatternRoute("/", NewRootHandler(cfg))
}
