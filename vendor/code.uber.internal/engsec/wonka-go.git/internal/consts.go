package internal

// internal constants for wonka will be defined here.

import "time"

const (
	// DerelictsCheckPeriod is how often to query wonkamaster for the services
	// that can bypass x-wonka-auth
	DerelictsCheckPeriod = 10 * time.Minute

	// MaxDisableDuration is the maximum amount of time that we accept a disable
	// message for.
	MaxDisableDuration = 24 * time.Hour

	// DisableCheckPeriod is how often to check if galileo is disabled.
	DisableCheckPeriod = time.Minute
)
