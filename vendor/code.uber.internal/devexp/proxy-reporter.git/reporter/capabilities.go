package reporter

type capabilities struct{}

// Reporting returns whether the reporter has the ability to actively report.
func (capabilities) Reporting() bool {
	return true
}

// Tagging returns whether the reporter has the capability for tagged metrics.
func (capabilities) Tagging() bool {
	return true
}
