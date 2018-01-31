package envfx

// This comes from code.uber.internal/go/envfx We internalize it here
// because the real envfx has a hard dependency on a version of uberfx
// which isn't yet fully deployed.
// When T1263419 landed, we can remove this.

import (
	"io/ioutil"
	"os"
)

const (
	_zoneKey  = "UBER_DATACENTER"
	_zoneFile = "/etc/uber/datacenter"
)

// Context is pulled straight from envfx
type Context struct {
	Zone string
}

// Result is pulled straight from envfx
type Result struct {
	Environment Context
}

// New is pulled straight from envfx
func New() Result {
	return Result{
		Environment: Context{
			Zone: getZone(),
		},
	}
}

// Read a value from the environment if possible, else fall back to a
// Puppet-managed file.
func readValue(envKey string, fileName string) (_ string, fromEnv bool) {
	if v, ok := os.LookupEnv(envKey); ok {
		return v, true
	}
	// N.B., these files don't have trailing newlines.
	if bs, err := ioutil.ReadFile(fileName); err == nil {
		return string(bs), false
	}
	return "", false
}

func getZone() string {
	val, _ := readValue(_zoneKey, _zoneFile)
	return val
}
