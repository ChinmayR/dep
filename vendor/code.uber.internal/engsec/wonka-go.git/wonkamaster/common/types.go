package common

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"
	"regexp"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// CertAuthOverride permits configuring overrides for how
// certificate authentication works.
type CertAuthOverride struct {
	Grant AuthGrantOverride
}

// AuthGrantOverride permits configuring more lenient certificate
// granting policies.
type AuthGrantOverride struct {
	SignedAfter  time.Time `yaml:"signed_after"`
	SignedBefore time.Time `yaml:"signed_before"`
	EnforceUntil time.Time `yaml:"enforce_until"`
}

// UnmarshalYAML allows the YAML to be unmarshalled into time.Time fields.
func (a *AuthGrantOverride) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal as strings and then convert
	type stringStruct struct {
		SignedAfter  string `yaml:"signed_after"`
		SignedBefore string `yaml:"signed_before"`
		EnforceUntil string `yaml:"enforce_until"`
	}

	temp := new(stringStruct)
	err := unmarshal(temp)
	if err != nil {
		return err
	}

	a.EnforceUntil, err = time.Parse(time.RFC3339, temp.EnforceUntil)
	if err != nil {
		return err
	}
	a.SignedAfter, err = time.Parse(time.RFC3339, temp.SignedAfter)
	if err != nil {
		return err
	}
	a.SignedBefore, err = time.Parse(time.RFC3339, temp.SignedBefore)
	return err
}

// HandlerConfig is the list of attributes needed by the various handlers to serve requests.
type HandlerConfig struct {
	Metrics        tally.Scope
	ECPrivKey      *ecdsa.PrivateKey
	RSAPrivKey     *rsa.PrivateKey
	Ussh           []ssh.PublicKey
	UsshHostSigner ssh.HostKeyCallback
	DB             wonkadb.EntityDB
	Pullo          rpc.PulloClient
	Imp            []string
	Logger         *zap.Logger
	Host           string
	// Derelicts is a map of service name and data (in YYYY/MM/DD format)
	// of services that are allowed to use old-timey x-uber-source auth.
	Derelicts                  map[string]string
	Launchers                  map[string]Launcher
	HoseCheckInterval          int
	CertAuthenticationOverride *CertAuthOverride
}

// Launcher is an entitiy that is allowed to rqeuest certificates for other tasks.
// The entity names allowed for a given task launcher are restricted by the
// AllowedTaskNames regexp. For instance, mesos might be allowed to launch *any* task,
// but piper can only launch tasks who's name starts with 'piper-'
type Launcher struct {
	LaunchedBy TaskRegexp `yaml:"launched_by"`
	LaunchedOn TaskRegexp `yaml:"launched_on"`
	TaskName   TaskRegexp `yaml:"taskname"`
}

// TaskRegexp is the task name regexp.
type TaskRegexp struct {
	*regexp.Regexp
}

// UnmarshalText unmarshals and validates a Regexp.
func (re *TaskRegexp) UnmarshalText(text []byte) error {
	r, err := regexp.Compile(string(text))
	if err != nil {
		return fmt.Errorf("error parsing regexp, %v: %v", text, err)
	}
	*re = TaskRegexp{r}
	return nil
}

// Router allows registering xhttp.Handlers.
type Router interface {
	AddPatternRoute(string, xhttp.Handler)

	// We use this interface so that users of handlers.SetupHandlers and
	// wonkatestdata.WithWonkaMaster are able to use both without referencing
	// internal/xhttp. This is needed to be able to use wonkatestdata from
	// outside the Wonka repo.
}

var _ Router = (*xhttp.Router)(nil)
