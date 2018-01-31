package galileo

import (
	"context"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const (
	// EveryEntity is the name of the special entity that allows requests from
	// anyone.
	EveryEntity = "EVERYONE"

	// WonkaRequiresHeader is the header in HTTP responses containing a
	// comma-separated list of entities that are allowed to make that request.
	WonkaRequiresHeader = "X-Wonka-Requires"

	// initialSkipDuration is how long we start to skip auth for an entity
	// when lookup for the entity fails.
	initialSkipDuration = time.Minute
	// maxSkipDuration is the longest we'll try to skip auth for an entity.
	maxSkipDuration = 30 * time.Minute
)

// Configuration of Galileo for ServiceName entity. Defines which entities are
// authorized for which endpoints, what to do on authorization failure, etc.
type Configuration struct {
	// ServiceName is the name of this Galileo entity. Outbound requests will be
	// authenticated to prove they come from this identity.
	ServiceName string `yaml:"servicename"`

	// ServiceAliases are alternative names for this service. Useful when your
	// service advertises as multiple names. Inbound requests will be required
	// to have a token with a destination from this list.
	// If ServiceAliases is not set, only tokens with destination equal to
	// ServiceName will be accepted.
	// When setting ServiceAliases, be sure to include ServiceName, for example:
	//   galileo:
	//     servicename: gopro
	//     servicealiases: [gopro, populous]
	ServiceAliases []string `yaml:"servicealiases"`

	// AllowedEntities is the list of entities who can make authenticated read
	// and write requests to all endpoints. Use Endpoints to override this
	// configuration for a specific endpoint.
	AllowedEntities []string `yaml:"allowedentities"`

	// Endpoints is endpoint-specific configuration for HTTP endpoints.
	// For each endpoint: specify the list of entities who can make
	// authenticated calls to it, and which HTTP verbs they can use.
	Endpoints map[string]EndpointCfg `yaml:"endpoints"`

	// EnforcePercentage allows partial enforcement of authentication.
	// 0.0 allows all requests, even requests with missing or invalid
	// authentication tokens.
	// 1.0 allows only requests with valid authentication tokens, according to
	// the endpoint configuration.
	// 0.X allows all requests with valid authentication tokens, and X% of
	// requests with missing or invalid authentication tokens.
	EnforcePercentage float32 `yaml:"enforce_percentage"`

	// PrivateKeyPath is the path to the file containing the RSA private key
	// that uniquely identifies this entity.
	PrivateKeyPath string `yaml:"privkeypath"`

	// Galileo will create a new metrics scope as a child of Metrics and tagged
	// with component:galileo. Optional. If omitted, no metrics will be sent.
	Metrics tally.Scope `yaml:"-"`
	// Send logs to this Logger after creating namespace `galileo`. Optional. If
	// omitted the global logger will be used.
	Logger *zap.Logger `yaml:"-"`

	// Tracer used to create and emit spans. If unset, this defaults to
	// opentracing.GlobalTracer.
	//
	// This MUST NOT be the opentracing.NoopTracer.
	Tracer opentracing.Tracer `yaml:"-"`

	// Disabled is a boolean flag to enable/disable galileo. If disabled galileo
	// will not check inbound auth baggage and will not try to decorate outbound
	// requests with auth baggage.
	Disabled bool `yaml:"disabled"`

	// Cache sets the caching configuration settings.
	Cache claimtools.CacheConfig `yaml:"cache"`
}

// EndpointCfg defines configuration for a specific HTTP endpoint: the list
// entities who can read, and the list of entities who can write.
type EndpointCfg struct {
	// AllowRead means GET, HEAD
	AllowRead []string `yaml:"allow_read"`
	// AllowWrite means POST, PUT, DELETE
	AllowWrite []string `yaml:"allow_write"`
}

// Galileo provides access to authentication for incoming and outgoing
// requests.
type Galileo interface {
	// Name returns the name of this Galileo entity.
	Name() string

	// Endpoint returns endpoint-specific configuration for the given HTTP
	// endpoint; entities who can read, and entities who can write.
	Endpoint(endpoint string) (EndpointCfg, error)

	// AuthenticateOut returns a copy of the given context with authentication
	// metadata attached to it. This allows a Galileo entity to authenticate
	// itself to the specified destination. For example, the following
	// requests any claim good for anotherservice.
	//
	//  ctx, err := g.AuthenticateOut(ctx, "anotherservice")
	//
	// Optionally, an explicit claim may be specified for the request. For
	// example, the following requests an AD:engineering claim with
	// anotherservice as the destination.
	//
	//  ctx, err := g.AuthenticateOut(ctx, "anotherservice", "AD:engineering")
	AuthenticateOut(ctx context.Context, destination string, explicitClaim ...interface{}) (context.Context, error)

	// AuthenticateIn validates the authentication information attached to the
	// provided context and verifies that the request should be allowed, based
	// on the Galileo configuration.
	//
	// Authentication baggage will be removed from the context.
	//
	// An error is returned if the request was unauthorized. Errors returned
	// by this function can be used with the GetAllowedEntities function to
	// determine the list of entities that would have been allowed to make
	// that request.
	//
	// Accepts zero or more CredentialValidationOption or string arguments.
	// Strings passed to AuthenticateIn are names of entities that are allowed
	// to make this request, instead of the entities configured with this
	// Galileo object.
	//
	//  if err := g.AuthenticateIn(ctx); err != nil {
	//    return fmt.Errorf("unauthorized request: %v", err)
	//  }
	//
	//  if err := g.AuthenticateIn(ctx, galileo.EveryEntity); err != nil {
	//    return fmt.Errorf("unauthorized request: %v", err)
	//  }
	//
	// See docs for individual CredentialValidationOptions for more options.
	AuthenticateIn(ctx context.Context, allowedEntities ...interface{}) error
}
