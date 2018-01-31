package wonka

import (
	"crypto"
	"crypto/ecdsa"
	"sync"
	"time"

	"code.uber.internal/engsec/wonka-go.git/redswitch"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// KeyType is type of private key used by an entity
type KeyType int

const (
	// KeyInvalid is an error
	KeyInvalid KeyType = iota
	// StaticKey means the private key is directly contained in the
	// configuration itself.
	StaticKey
	// UsshKey is a key based on the ussh user certificate.
	UsshKey
	// CertificateKey is a wonka Certificate and key.
	CertificateKey
	// FileKey means configuration contained a path to a file containing the
	// key. Possibly encoded in a yaml file loaded from langley.
	FileKey
)

type entityKey struct {
	ctime time.Time
	key   *ecdsa.PublicKey
}

// uberWonka implements the wonka interface
type uberWonka struct {
	log     *zap.Logger
	metrics tally.Scope
	tracer  opentracing.Tracer

	entityName string

	clientKey crypto.PrivateKey
	ussh      *ssh.Certificate

	// access to these two must be protected
	clientECC    *ecdsa.PrivateKey
	certificate  *Certificate
	clientKeysMu sync.RWMutex

	sshAgent agent.Agent

	cachedKeys   map[string]entityKey
	cachedKeysMu sync.RWMutex

	httpRequester     *httpRequester
	wonkaURLRequested string

	// we can use a sync.Map eventually
	derelicts              map[string]time.Time
	derelictsTimer         *time.Timer
	derelictsRefreshPeriod time.Duration
	derelictsLock          sync.RWMutex

	// globally disabled switch
	globalDisableReader   redswitch.CachelessReader
	globalDisableRecovery chan time.Time

	implicitClaims []string

	// cancel is called when the user calls wonka.Close(w)
	cancel func()

	// certRepository is a reference to the global certificate
	// repository.
	certRepository certificateRegistry

	// certRegHandle is the handle returned from the certRepository
	// after we have registered our certificate with it. It is required
	// for reading updates and for eventual unregistration.
	certRegHandle *certificateRegistrationHandle
}

// SecretsYAML has the wonka public and private keys.
type SecretsYAML struct {
	WonkaPrivate string `yaml:"wonka_private"`
	WonkaPublic  string `yaml:"wonka_public"`
}

// Config is a wonka configuration struct.
type Config struct {
	// EntityName is the name of this particular wonka entity.
	EntityName string
	// EntityLocation is deprecated. It used to describe where an entity
	// could be found, if wonkamster were acting as a directory service.
	// Deprecated: EntityLocation is unused.
	EntityLocation string
	// PrivateKeyPath is either the pem bytes of the rsa private key, or
	// where the private key can be found. If it starts with a "/", it's
	// it's treated as a path, otherwise it's treated as pem bytes.
	// currently: "/langley/current/<entity name>/wonka_private.yaml"
	PrivateKeyPath string
	// Deprecated: DestinationRequires is unused.
	DestinationRequires []string
	// Wonka client will create a new metrics scope as a child of Metrics and
	// tagged with component:wonka. Optional.
	Metrics tally.Scope
	// Send logs to this Logger with field component:wonka. Optional.
	Logger *zap.Logger
	// Tracer used to create and emit spans. If unset, this defaults to
	// opentracing.GlobalTracer.
	Tracer opentracing.Tracer
	// Agent is an ssh agent.
	Agent agent.Agent
	// WonkaMasterURL is the preferred protocol:hostname:port used for calls to the Wonka Master.
	WonkaMasterURL string
	// ImplicitClaims are claims that we always ask for.
	ImplicitClaims []string
	// WonkaClientCert is a wonka certificate
	WonkaClientCert *Certificate
	// WonkaClientKey is a wonka key
	WonkaClientKey *ecdsa.PrivateKey
	// WonkaMasterPublicKeys is a slice of public keys for wonkamaster
	WonkaMasterPublicKeys []*ecdsa.PublicKey

	// WonkaClientCertPath is the file path of the wonka client certificate.
	// The file should contain a JSON-encoded Certificate type as defined below.
	//
	// Note: This option is prioritized over the WONKA_CLIENT_CERT environment variable.
	WonkaClientCertPath string

	// WonkaClientKeyPath is the file path of the wonka client key.
	// The file should contain a DER-encoded EC private key, further encoded into base64.
	//
	// Note: This option is prioritized over the WONKA_CLIENT_KEY environment variable.
	WonkaClientKeyPath string
}

// EntityType describes the type of entity for which a claim or certificate
// was intended. It's only incorporated into the certificates for now, but it
// will eventually find its way into the claim requests.
type EntityType int

const (
	// EntityTypeInvalid represents an error finding the entity type.
	// Claims and certificates cannot be issued.
	EntityTypeInvalid EntityType = iota
	// EntityTypeService is a service, eg rt-api
	EntityTypeService
	// EntityTypeUser is an employee, eg pmoody@uber.com
	EntityTypeUser
	// EntityTypeHost is a host, eg engsec01-sjc1.prod.uber.internal
	EntityTypeHost
)

// Certificate identifies any entity in the wonka ecosystem, signed by wonkamaster.
//
// Each instance of a service will have its own private/public key-pair.
// This means that a Certificate uniquely identifies one particular
// execution of the service on a particular host.
type Certificate struct {
	// EntityName is the fully qualified name of the service, user, or host.
	EntityName string `json:"entity_name,omitempty"`
	// Type indicates whether the entity is a service, user, host, etc.
	Type EntityType `json:"entity_type,omitempty"`
	// Host is the name of the underlying machine (e.g. appdocker01-sjc1.prod.uber.internal
	// or adhoc01-dca1.prod.uber.internal) where the service is running.
	Host string `json:"host,omitempty"`
	// Key is the serialized public key (DER-encoded PKIX format) associated with the service's
	// private key.
	Key []byte `json:"key,omitempty"`
	// Serial is a randomly-generated, unique serial number to identify
	// the certificate. Useful for tracking.
	Serial uint64 `json:"serial,omitempty"`
	// ValidAfter is the unix timestamp (in epoch seconds) after which the cert becomes valid.
	ValidAfter uint64 `json:"valid_after,omitempty"`
	// ValidBefore is the unix timestamp (in epoch seconds) after which the cert expires.
	ValidBefore uint64 `json:"valid_before,omitempty"`
	// Tags is an arbitrary set of key-values. Useful for tracing or other metadata.
	Tags map[string]string `json:"tags,omitempty"`
	// Signature is a signature calculated over the certificate using wonkamaster's private key.
	Signature []byte `json:"signature,omitempty"`
}

// Cookie is a wonka authentication cookie. It's signed by the private key associated with
// the included Certificate. The certificate chains back to wonka master.
type Cookie struct {
	Destination string       `json:"d,omitempty"`
	Version     string       `json:"v,omitempty"`
	Serial      uint64       `json:"s,omitempty"`
	Certificate *Certificate `json:"c,omitempty"`
	Ctime       uint64       `json:"t,omitempty"`
	Signature   []byte       `json:"sig,omitempty"`
}

// CertificateSignature is signature that includes the cert that
// signed some data.
type CertificateSignature struct {
	Certificate Certificate `json:"certificate"`
	Timestamp   int64       `json:"timestamp"`
	Data        []byte      `json:"data"`
	Signature   []byte      `json:"signature,omitempty"`
}

// LaunchRequest is a launch request from the mesos scheduler
type LaunchRequest struct {
	// this is in both mesos and docker
	Hostname string `json:"hostname,omitempty"`
	SvcID    string `json:"svc_id,omitempty"`

	// these are in only mesos
	TaskID    string `json:"task_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`

	// these are in only docker
	InstID string `json:"inst_id,omitempty"`
}

// WonkadRequest is how a process communicates with the local wonka daemon.
type WonkadRequest struct {
	Process       string      `json:"process,omitempty"`
	Service       string      `json:"service.omitempty"`
	TaskID        string      `json:"taskid,omitempty"`
	Destination   string      `json:"destination,omitempty"`
	Certificate   Certificate `json:"certificate,omitempty"`
	LaunchRequest []byte      `json:"launch_request,omitempty"`
	Signature     []byte      `json:"signature,omitempty"`
}

// WonkadReply is how wonkad replies with a certificate and private key.
type WonkadReply struct {
	Certificate []byte `json:"certificate"`
	PrivateKey  []byte `json:"privatekey"`
}

// CertificateSigningRequest is both the wonka certificate signing
// request and the reply. For successful replies, the certificate has
// the Signature field set by the wonkamaster and Result ==
// StatusOK. If there's an error, the certificate isn't included in
// the reply and Result is set to some error code.
type CertificateSigningRequest struct {
	Certificate        []byte `json:"certificate,omitempty"`
	LaunchRequest      []byte `json:"launch_request,omitempty"`
	SigningCertificate []byte `json:"signing_certificate,omitempty"`
	USSHCertificate    []byte `json:"ussh_certificate,omitempty"`
	Signature          []byte `json:"signature,omitempty"`
	SignatureType      string `json:"signature_type,omitempty"`
	Result             string `json:"result,omitempty"`
}

// ClaimRequest is sent to the wonkamaster to request a particular claim.
// TODO(pmoody): add entity type to this when we figure out the proper usage.
type ClaimRequest struct {
	Version            string `json:"version,omitempty"`
	EntityName         string `json:"entity_name,omitempty"`
	ImpersonatedEntity string `json:"impersonated_entity,omitempty"`
	Claim              string `json:"claim_request,omitempty"`
	Ctime              int64  `json:"ctime,omitempty"`
	Etime              int64  `json:"etime,omitempty"`
	Destination        string `json:"destination,omitempty"`
	Signature          string `json:"entity_signature,omitempty"`
	SigType            string `json:"entity_sigtype,omitempty"`
	SessionPubKey      string `json:"session_pubkey,omitempty"`
	USSHCertificate    string `json:"ussh_certificate,omitempty"`
	USSHSignature      string `json:"ussh_signature,omitempty"`
	USSHSignatureType  string `json:"ussh_sigtype,omitempty"`
	Certificate        []byte `json:"certificate,omitempty"`

	// TODO(pmoody): remove these fields
	CreateTime time.Time `json:"-"`
	ExpireTime time.Time `json:"-"`
}

// ClaimResponse is the reply from the wonkamaster for a claim request.
// Token is encrypted with either the entity public key or the session publickey
type ClaimResponse struct {
	Result string `json:"result"`
	Token  string `json:"claim_token"`
}

// Claim is the decoded claim token.
// TODO(pmoody): add entity type to this when we figure out the proper usage.
type Claim struct {
	ClaimType   string   `json:"ct"`
	ValidAfter  int64    `json:"va"`
	ValidBefore int64    `json:"vb"`
	EntityName  string   `json:"e"`
	Claims      []string `json:"c"`
	Destination string   `json:"d"`
	Signature   []byte   `json:"s"`
}

// Entity is a wonka entity.
type Entity struct {
	EntityName      string `json:"entity_name,omitempty"`
	Requires        string `json:"requires,omitempty"`
	PublicKey       string `json:"public_key,omitempty"`
	Version         string `json:"version,omitempty"`
	ECCPublicKey    string `json:"ecc_key,omitempty"`
	USSHCertificate string `json:"ussh_certificate,omitempty"`
	Ctime           int    `json:"ctime,omitempty"`
	Etime           int    `json:"etime,omitempty"`
	SigType         string `json:"entity_sigtype,omitempty"`
	EntitySignature string `json:"entity_signature,omitempty"`

	// these fields should be removed
	IsLocked   int       `json:"is_locked,omitempty"`
	KeyBits    int32     `json:"keybits"`
	ExpireTime time.Time `json:"-"`
	CreateTime time.Time `json:"-"`
	Algo       string    `json:"algo,omitempty"`
	Location   string    `json:"location,omitempty"`
}

// EnrollRequest is a request to enroll an entity. If it's a new enrollment, it
// must be accompanied by a claim.
type EnrollRequest struct {
	Entity *Entity `json:"entity"`
	Claim  *Claim  `json:"claim"`
}

// EnrollResponse is a response to an enroll message.
type EnrollResponse struct {
	Result string `json:"result"`
	Entity Entity `json:"entity,omitempty"`
}

// AdminRequest is a request for an admin action.
type AdminRequest struct {
	EntityName      string      `json:"entity_name,omitempty"`
	Version         int         `json:"version,omitempty"`
	Action          AdminAction `json:"admin_action,omitempty"`
	ActionOn        string      `json:"action_on,omitempty"`
	Ctime           int64       `json:"ctime,omitempty"`
	Ussh            string      `json:"ussh,omitempty"`
	Signature       string      `json:"signature,omitempty"`
	SignatureFormat string      `json:"signature_format,omitempty"`
}

// GenericResponse is for all handlers that just returns a result string.
type GenericResponse struct {
	Result string `json:"result"`
}

// TheHoseRequest is a request to the hose endpoint. It just identifies the caller.
type TheHoseRequest struct {
	Ctime      int64  `json:"ctime,omitempty"`
	EntityName string `json:"entity_name,omitempty"`
	Signature  []byte `json:"signature,omitempty"`
}

// TheHoseReply is a signed status message from the wonkamaster. Derelicts are services
// which can continue to rely on x-uber-source. The time field is the date, in YYYY/MM/DD
// format, when their exception status expires.
type TheHoseReply struct {
	CurrentStatus string               `json:"current_status,omitempty"`
	CurrentTime   int64                `json:"current_time,omitempty"`
	Derelicts     map[string]time.Time `json:"derelicts,omitempty"`
	Signature     []byte               `json:"signature,omitempty"`
	CheckInterval int                  `json:"check_interval,omitempty"`
}

// LookupRequest is a lookup request. The format of a lookup request signature is:
//
// Service Lookup:
//   SHA256(EntityName<Ctime>RequestedEntity) | signed by private key
//
// Personnel Lookup: (nb: private kye is a session key, it can be per request)
//   SHA1(EntityName<Ctime>RequestedEntity|USSHCertificate) | signed by ussh cert
type LookupRequest struct {
	Version         string `json:"version,omitempty"`
	Ctime           int    `json:"ctime,omitempty"`
	EntityName      string `json:"entity_name,omitempty"`
	RequestedEntity string `json:"requested_entity,omitempty"`
	Signature       string `json:"signature,omitempty"`
	SigType         string `json:"sigtype,omitempty"`
	// session stuff
	USSHSignatureType string `json:"ussh_sigtype,omitempty"`
	USSHSignature     string `json:"ussh_signature,omitempty"`
	USSHCertificate   string `json:"ussh_certificate,omitempty"`
	Certificate       []byte `json:"certificate,omitempty"`
}

// LookupResponse is the wonkmaseter reply to a Lookup() request
type LookupResponse struct {
	Result    string `json:"result"`
	Entity    Entity `json:"wonka_entity"`
	Signature string `json:"master_signature"`
}

// ResolveRequest is claim request good for the named entity, if it's enrolled.
// If it's not enrolled, it returns an EVERYONE claim.
type ResolveRequest struct {
	EntityName        string `json:"entity_name"`
	RequestedEntity   string `json:"requested_entity"`
	Claims            string `json:"claims,omitempty"`
	Certificate       []byte `json:"certificate,omitempty"`
	PublicKey         string `json:"publickey,omitempty"`
	Etime             int64  `json:"etime,omitempty"`
	USSHSignatureType string `json:"ussh_sigtype,omitempty"`
	USSHCertificate   []byte `json:"ussh_certificate,omitempty"`
	Signature         []byte `json:"signature,omitempty"`
}

// DisableMessage is signed by wonkamaster and put as a txt record in a json blob
// under uber.com. It's used to globally disable wonka for some period of time.
// This is a hack and will go away when flipr supports disabling galileo.
//
// Deprecated: use redswitch.DisableMessage instead.
type DisableMessage struct {
	Ctime      int64  `json:"ctime,omitempty"`
	Etime      int64  `json:"etime,omitempty"`
	IsDisabled bool   `json:"is_disabled,omitmpty"`
	Signature  []byte `json:"signature,omitempty"`
}

// AdminAction is a particular administrative action.
type AdminAction string

const (
	// DeleteEntity requests to delete an entity
	DeleteEntity AdminAction = "Delete"
)

const (
	rsaPrivHeader = "-----BEGIN RSA PRIVATE KEY-----"
	rsaPrivFooter = "-----END RSA PRIVATE KEY-----"

	// WonkaHeader is the name of the wonka-auth header.
	WonkaHeader = "X-Wonka-Auth"
)

// the following are errors shared between wonka-go and wonka-master
const (
	// DecodeError means the request json failed to parse
	DecodeError = "DECODE_ERROR"
	// EntityUnknown is returned when the requested entity isn't known to wonkamaster.
	EntityUnknown = "ENTITY_UNKNOWN"
	// ResultRejected is a generic rejected
	ResultRejected = "REJECTED"
	// ResultOK is a generic OK
	ResultOK = "OK"
	// SignatureVerifyError is returned when the signature is invalid
	SignatureVerifyError = "VERIFY_ERROR"
	// InvalidPublicKey means keys don't match
	InvalidPublicKey = "REJECTED_INVALID_PUBLICKEY"
	// InternalError can be returned when wonkamaster barfs for some reason
	InternalError = "WONKA_INTERNAL_ERROR"
	// GatewayError is returned when one of wonkamaster's service dependencies is unavailable
	GatewayError = "WONKA_GATEWAY_ERROR"
	// ErrTimeWindow can be returned when there is an error with the time stamp on the request
	// being outside of the allowed skew.
	ErrTimeWindow = "ERROR_OUTSIDE_TIME_WINDOW"

	// AdminAccessDenied is returned when the requestor doesn't have sufficient permissions
	// to perform the requested action.
	AdminAccessDenied = "ERROR_ACCESS_DENIED"
	// AdminInvalidCmd is returned when a non-supported command is requested.
	AdminInvalidCmd = "ERROR_INVALID_COMMAND"
	// AdminInvalidID is returned when the request is missing a target entity.
	AdminInvalidID = "ERROR_INVALID_ID"
	// AdminUnknownEntity is returned when the requested entity isn't known to wonkamaster.
	AdminUnknownEntity = EntityUnknown
	// AdminSuccess is returned when the admin action completed successfully.
	AdminSuccess = ResultOK

	// ClaimEntityUnknown is returned when the entity requesting a claim isn't known to wonkamaster
	ClaimEntityUnknown = EntityUnknown
	// ClaimVerifyError is returned when the claim signature is invalid
	ClaimVerifyError = SignatureVerifyError
	// ClaimRequestNotYetValid is for when a claim request isn't valid yet
	ClaimRequestNotYetValid = "CLAIM_REQUEST_NOT_YET_VALID"
	// ClaimRequestExpired is returned when the claim request has expired
	ClaimRequestExpired = "CLAIM_REQUEST_EXPIRED"
	// ClaimRejectedNoAccess is returned when the entity making the request isn't permitted
	// to get the requested claim.
	ClaimRejectedNoAccess = "REJECTED_CLAIM_NO_ACCESS"
	// ClaimSigningError is returned when wonkamaster encounters an error generating a signed claim.
	ClaimSigningError = "ERROR_SIGNING_CLAIM"
	// ClaimInvalidImpersonator is returned when an entity attempts to perform an unauthorized impersonation request
	ClaimInvalidImpersonator = "REJECTED_INVALID_IMPERSONATOR"

	// EnrollNotYetValid means the enrollment request isn't valid yet.
	EnrollNotYetValid = "REJECTED_ENROLL_NOT_YET_VALID"

	// EnrollExpired means the enrollment message has expired.
	EnrollExpired = "REJECTED_ENROLLMENT_EXPIRED"
	// EnrollInvalidTime means that the request expires before it was created.
	EnrollInvalidTime = "REJECTED_INVALID_TIME"
	// EnrollInvalidEntity means the EntityName or Location contain invalid characters.
	EnrollInvalidEntity = "REJECTED_INVALID_ENTITY_NAME"
	// EnrollNotPermitted means the enroll or update request isn't permitted, usually
	// because the entity keys don't match or the enrollment request is coming from a
	// non-galileo enable connection.
	EnrollNotPermitted = "REJECTED_ENROLL_NOT_PERMITTED"
	// InvalidPublicKey means the publickey failed to parse.
	EnrollInvalidPublicKey = InvalidPublicKey
	// VerifyError means the signature didn't match
	EnrollVerifyError = SignatureVerifyError

	// LookupServerError is returned when the wonka master encounters some sort of
	// db error when looking up and entity.
	LookupServerError = "LOOKUP_SERVER_ERROR"
	// LookupInvalidUSSHCert is returned when the lookup request has an invalid
	// ussh certificate attached
	LookupInvalidUSSHCert = "REJECTED_INVALID_USSH_CERT"
	// LookupInvalidUSSHSignature is returned when the ussh signature is invalid.
	LookupInvalidUSSHSignature = "REJECTED_INVALID_USSH_SIGNATURE"
	// LookupExpired is returned when the lookup request has expired.
	LookupExpired = "REJECTED_LOOKUP_EXPIRED"
	// LookupEntityUnknown is returned when the requesting entity isn't known to wonkamaster
	LookupEntityUnknown = EntityUnknown
	// LookupInvalidSignature is returned when the lookup request has an invalid signature
	LookupInvalidSignature = SignatureVerifyError

	// BadCertificateSigngingRequest is a general CSR error.
	BadCertificateSigningRequest = "INVALID_CERTIFICATE_SIGNING_REQUEST"
	// CSRNotYetValid is returend when the CSR is from the futrue
	CSRNotYetValid = "CERTIFICATE_SIGNING_REQUEST_NOT_YET_VALID"
	// CSRExpired is returned when the CSR has expired.
	CSRExpired = "CERTIFICATE_SIGNING_REQUEST_EXPIRED"
)

const (
	// SHA256 is the sha256 hashing algorithm. This is the default.
	// ussh-based signatures use whatever hashing algorithm is the default
	// for the particular key type. our ed25519 keys use sha-512, but that
	// info is stored in the ssh signature message so we don't declare
	// a const for it.
	SHA256 = "SHA256"

	// EntityVersion is the standard version string sent with a wonka Entity.
	EntityVersion = "1.0.0"
	// SignEverythingVersion tells wonkamaster what sort of request this is. This is
	// to facilitate otherwise backwards incompatible changes.
	SignEverythingVersion = "2.0.0"

	// EveryEntity refers to every entity that is authenticated to the wonkamaster
	// ecosystem.
	EveryEntity = "EVERYONE"
	// NullEntity is the empty entity. Nothing can get a claim for the NullEntity and
	// nothing matches the NullEntity.
	NullEntity = "NULLENTITY"

	// EnrollerGroup refers to the active directory group who is authorized to
	// enroll new entities into wonkamaster.
	EnrollerGroup = "AD:engineering"

	// TagTaskID is the id of the particular task.
	TagTaskID = "TaskID"
	// TagRuntime is the runtime environment of a given task, for example "production" or "staging".
	TagRuntime = "Runtime"
	// TagUSSHCert is the ussh cert of the host where a given task was launched.
	TagUSSHCert = "USSHCertificate"
	// TagLaunchRequest is the launch request for a given task.
	TagLaunchRequest = "LaunchRequest"
)

var (
	// ECCPUB is the compressed wonkamaster public key
	ECCPUB = "0227df5d243251693b394971eafecdb05f3ac721082718f45d83a272c94cff2c1d"

	// WonkaMasterPublicKeys is the list of public keys good for wonkamaster.
	WonkaMasterPublicKeys []*ecdsa.PublicKey
	// WonkaMasterPublicKey is the public key of the wonkamaster. It is the first
	// key from the WonkaMasterPublicKeys slice. It's here for backwards compatibility
	WonkaMasterPublicKey *ecdsa.PublicKey

	// certRefreshPeriod
	certRefreshPeriod = 30 * time.Minute

	// WonkadTCPAddress is the network listener for wonkad.
	// TODO(pmoody): talk to DRI to get an actual port.
	WonkadTCPAddress = "localhost:777"
)
