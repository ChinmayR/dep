package wonka

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

// Wonka interface, useful for mocking out wonka.
type Wonka interface {
	// Admin makes an admin request. The type of request is defined by the AdminAction set
	// in the request struct. The signature is filled in by wonka.
	Admin(ctx context.Context, request AdminRequest) error

	// CSR sends a wonka certificate to the wonkamster for signing
	CertificateSignRequest(ctx context.Context, cert *Certificate, req *CertificateSignature) error

	// ClaimRequest will request a token affirming the given claim, with the given dest.
	ClaimRequest(ctx context.Context, claim, dest string) (*Claim, error)

	// ClaimRequestTTL will request a token affirming the given claim, with the given dest,
	// and valid for duration explicitly requested in ttl.
	ClaimRequestTTL(ctx context.Context, claim, dest string, ttl time.Duration) (*Claim, error)

	// ClaimResolve will try to request a claim good for the named entity.
	ClaimResolve(ctx context.Context, entityName string) (*Claim, error)

	// ClaimResolve will try to request a claim good for the named entity.
	//ClaimResolveNew(ctx context.Context, entityName string) (*Claim, error)

	// ClaimResolveTTL will try to request a claim good for the named entity,
	// explicitly requesting a TTL of ttl.
	ClaimResolveTTL(ctx context.Context, entityName string, ttl time.Duration) (*Claim, error)

	// Sign signs data with the entities derived ecc private key. The Data
	// is hashed with sha256.
	Sign(data []byte) ([]byte, error)

	// Verify verifies that data was signed by private ecc key held
	// by the named entity. If needed, it will contact wonkamaster to get the
	// publickey for the named entity and cache the result.
	Verify(ctx context.Context, data, sig []byte, entity string) bool

	// Encrypt encrypts data for entity.
	Encrypt(ctx context.Context, plainText []byte, entity string) ([]byte, error)

	// Decrypt decrypts data encrypted by entity.
	Decrypt(ctx context.Context, cipherText []byte, entity string) ([]byte, error)

	// Enroll enrolls an entity with the with wonka master
	Enroll(ctx context.Context, location string, claims []string) (*Entity, error)

	// EnrollEntity enrolls an entity with the with wonka master
	EnrollEntity(ctx context.Context, entity *Entity) (*Entity, error)

	// ClaimImpersonateTTL will try to request a claim on behalf of impersonatedEntity
	// TODO(pmoody): this should be merged with the regular claim request methods.
	ClaimImpersonateTTL(ctx context.Context, impersonatedEntity string, entityName string, ttl time.Duration) (*Claim, error)

	// Ping tests that we can connect to the wonkamaster.
	Ping(ctx context.Context) error

	// Lookup returns the wonka entity associated with the entity, if it exists.
	Lookup(ctx context.Context, entityName string) (*Entity, error)

	// EntityName returns the EntityName.
	EntityName() string

	// LastError is here for libwonka compatibility. With libwonka, it behaves
	// similar to perror(3)
	LastError() string
}

// Crypter is a superset of the Wonka interface with also provides methods
// for performing cryptographic operations utilizing the entity's private key.
//
// This can be used by type asserting wonka.EntityCrypter on the Wonka
// instance returned from wonka.Init.
//
//   w, err := wonka.Init(cfg)
//   entityCrypter := w.(wonka.EntityCrypter).NewEntityCrypter()
//
type Crypter interface {
	Wonka

	// Certificate returns the certificate used by Wonka for authentication
	// if one exists.
	Certificate() *Certificate

	// NewEntityCrypter returns a new instance of wonkacrypter.EntityCrypter that
	// utilizes this instance of Wonka.
	//
	// Will return a nil instance if an EntityCrypter cannot be instantiated.
	NewEntityCrypter() wonkacrypter.EntityCrypter
}

// Closer is a superset of the Wonka interface which provides a Close() method.
// wonka.Close(w) should be called when a object is no longer needed. This is
// necessary because wonka.Init() kicks off goroutines for maintaining state and
// those goroutines need to be terminated.
type Closer interface {
	Wonka
	Close() error
}

var _ Closer = (*uberWonka)(nil)

var (
	// ErrNoEntity is returned when Init() is called without an entity name.
	ErrNoEntity = errors.New("no entity name provided")
	// ErrHomeless is called when Init() is called with no home directory.
	ErrHomeless = errors.New("no home directory provided")
	// ErrEccPoint is returned if there's an error loading one of the client
	// ECC points.
	ErrEccPoint = errors.New("couldn't set an ecc point")
	// ErrNoKey is returned when an operation that requires a private key is called
	// when the private key doesn't exist.
	ErrNoKey = errors.New("no rsa key found")
	// ErrBadClaim is returned when the claim is malformed.
	ErrBadClaim = errors.New("bad claim")

	// keySize is the size of the rsa key in bits.
	keySize = 2048
)

var _ Crypter = (*uberWonka)(nil)

// init is called when wonka-go is imported. It just initializes the server key
// at this point.
func init() {
	log := zap.L().With(zap.Namespace("wonka"))
	if err := InitWonkaMasterECC(); err != nil {
		log.Error("error initializing server publickey", zap.Error(err))
	}
}

// InitWonkaMasterECC initializes the wonkamaster public key.
func InitWonkaMasterECC() error {
	log := zap.L().With(zap.Namespace("wonka"))

	WonkaMasterPublicKeys = make([]*ecdsa.PublicKey, 0, 1)
	masterECC := ECCPUB

	if envC := os.Getenv("WONKA_MASTER_ECC_PUB"); envC != "" {
		log.Debug("compressed wonka master key")
		masterECC = envC
	}

	for _, k := range strings.Split(masterECC, ",") {
		pubKey, err := KeyFromCompressed(k)
		if err != nil {
			log.Error("invalid compressed key", zap.Error(err))
			continue
		}
		WonkaMasterPublicKeys = append(WonkaMasterPublicKeys, pubKey)
	}

	if len(WonkaMasterPublicKeys) == 0 {
		return errors.New("no wonkamaster public keys")
	}

	WonkaMasterPublicKey = WonkaMasterPublicKeys[0]

	return nil
}

// Init returns a new wonka object and uses the Background context for health
// checks. If you call this, you almost definitely need to call `wonka.Close(w)`
// on the returned wonka object when you're done. See the comment below on
// InitWithContext for more information.
func Init(cfg Config) (Wonka, error) {
	return InitWithContext(context.Background(), cfg)
}

// InitWithContext returns a new wonka object and uses the provided context to
// for health checks to determine how to contact wonkamaster.  If you are
// creating lots of wonka objects, you should either be passing in a cancelable
// context and calling cancel or calling `wonka.Close(w)` on the return Wonka
// object. Failure to do so will result in timers leaking (this was the cause of
// a wonka-outage that nearly impacted user-facing frontends).
func InitWithContext(ctx context.Context, cfg Config) (Wonka, error) {
	if cfg.EntityName == "" {
		return nil, ErrNoEntity
	}

	// init() should have been called before we get here, so if
	// WonkaMasterPublicKeys isn't set, there was an error
	if len(WonkaMasterPublicKeys) == 0 {
		return nil, ErrEccPoint
	}

	w := &uberWonka{
		entityName:         cfg.EntityName,
		cachedKeys:         make(map[string]entityKey, 0),
		cachedKeysMu:       &sync.RWMutex{},
		clientKeysMu:       &sync.RWMutex{},
		sshAgent:           cfg.Agent,
		implicitClaims:     cfg.ImplicitClaims,
		isGloballyDisabled: atomic.NewBool(false),
		derelicts:          make(map[string]time.Time),
		derelictsLock:      &sync.RWMutex{},
	}

	ctx, w.cancel = context.WithCancel(ctx)

	if err := w.initLogAndMetrics(cfg); err != nil {
		return nil, err
	}
	w.initTracerAndHTTPClient(cfg)

	keyType, err := w.loadKey(ctx, cfg)
	if err != nil {
		return nil, err
	}
	w.log.Debug("found a key", zap.Any("keytype", keyNameFromType(keyType)))

	for idx, k := range WonkaMasterPublicKeys {
		w.log.Debug("server ecc key",
			zap.Any("key number", idx),
			zap.Any("compressed", KeyToCompressed(k.X, k.Y)))
	}

	w.destRequires = []string{w.entityName}
	if cfg.DestinationRequires != nil {
		w.destRequires = cfg.DestinationRequires
	}

	w.setWonkaURL(ctx, cfg.WonkaMasterURL)
	w.log.Debug("wonka client initialized",
		zap.Any("url", w.wonkaURL),
		zap.Any("version", Version),
	)

	go w.checkGlobalDisableStatus(ctx, WonkaMasterPublicKeys)
	go w.checkDerelicts(ctx, internal.DerelictsCheckPeriod)

	w.metrics.Tagged(map[string]string{
		"stage": "initialized",
	}).Counter("wonka-go").Inc(1)

	return w, nil
}

// Close actually kills the background goroutines.
func (w *uberWonka) Close() error {
	if w.cancel != nil {
		w.cancel()
	}
	return nil
}

// Close terminates all of the background goroutines.
func Close(w Wonka) error {
	closer, ok := w.(Closer)
	if !ok {
		return errors.New("wonka instance must implement wonka.Closer interface")
	}
	return closer.Close()
}

// todo(pmoody): consider removing the key and cert from disk.
func (w *uberWonka) loadKeysFromEnv() error {
	clientCert := os.Getenv("WONKA_CLIENT_CERT")
	clientKey := os.Getenv("WONKA_CLIENT_KEY")
	if clientKey == "" || clientCert == "" {
		return errors.New("WONKA_CLIENT_CERT and/or WONKA_CLIENT_KEY not set")
	}

	certBytes, err := ioutil.ReadFile(clientCert)
	if err != nil {
		return fmt.Errorf("error reading certfile: %v", err)
	}

	cert, err := UnmarshalCertificate(certBytes)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	clientKeyBytes, err := ioutil.ReadFile(clientKey)
	if err != nil {
		return fmt.Errorf("error reading client key: %v", err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(string(clientKeyBytes))
	if err != nil {
		return fmt.Errorf("error decoding private key: %v", err)
	}

	key, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return fmt.Errorf("error unmarshalling private key: %v", err)
	}

	w.writeCertAndKey(cert, key)
	return nil
}

// askWonkadForKeys asks wonkad for a key and cert. normally this needs to be a signed
// request, but that requires mesos and/or docker to bootstrap the key and cert.
func (w *uberWonka) askWonkadForKeys() error {
	name := w.entityName

	if id := os.Getenv("SVC_ID"); id != w.entityName {
		w.log.Info("svc_id != entityName",
			zap.Any("svc_id", id),
			zap.Any("entity_name", w.entityName),
		)
	}

	taskID := os.Getenv("MESOS_EXECUTOR_ID")
	if taskID == "" {
		taskID = os.Getenv("UDEPLOY_INSTANCE_NAME")
	}

	w.log.Debug("certificate request",
		zap.Any("service", name),
		zap.Any("taskid", taskID))

	req := WonkadRequest{
		Service: name,
		TaskID:  taskID,
	}

	toWrite, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshalling msg to send to wonkad: %v", err)
	}

	conn, err := net.Dial("tcp", WonkadTCPAddress)
	if err != nil {
		return fmt.Errorf("error connecting to wonkad: %v", err)
	}

	if _, err := conn.Write(toWrite); err != nil {
		return fmt.Errorf("error writing request to wonkad: %v", err)
	}

	b, err := ioutil.ReadAll(conn)
	if err != nil {
		return fmt.Errorf("error reading reply from wonkad: %v", err)
	}

	var repl WonkadReply
	if err := json.Unmarshal(b, &repl); err != nil {
		return fmt.Errorf("error unmarshalling reply from wonkad: %v", err)
	}

	cert, err := UnmarshalCertificate(repl.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	key, err := x509.ParseECPrivateKey(repl.PrivateKey)
	if err != nil {
		return fmt.Errorf("error parsing ec private key: %v", err)
	}

	w.clientKeysMu.Lock()
	w.certificate = cert
	w.clientECC = key
	w.clientKeysMu.Unlock()

	return nil
}

func (w *uberWonka) loadKey(ctx context.Context, cfg Config) (KeyType, error) {
	// first check to see if we have a key in the environment
	envErr := w.loadKeysFromEnv()
	if envErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"),
			zap.String("source", "env"),
		)

		go w.refreshWonkaCert(ctx, certRefreshPeriod)
		return CertificateKey, nil
	}
	w.log.Info("unable to load private key from env, trying to load from config.", zap.Error(envErr))

	// next check to see if we have a configured key
	var cfgErr error
	w.clientKey, cfgErr = w.loadPrivateKey(cfg.PrivateKeyPath)
	if cfgErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "static"),
			zap.String("source", "config"),
			zap.String("path", cfg.PrivateKeyPath),
		)
		return StaticKey, w.initClientECC()
	}

	w.log.Info("unable to read private key, trying personnel request.",
		zap.Error(cfgErr),
		zap.Any("sshAgent_is_nil", w.sshAgent == nil),
	)

	var usshErr error
	w.ussh, usshErr = w.usshUserCert()
	if usshErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "ussh"),
			zap.String("source", "ussh"),
		)
		if w.ussh.CertType == ssh.UserCert {
			// UBER_OWNER is set on laptops
			if userName := os.Getenv("UBER_OWNER"); userName != "" {
				if !strings.EqualFold(w.entityName, userName) {
					w.log.Warn("entityname != ussh name. Changing entity name to match, see http://t.uber.com/wm-ipen",
						zap.String("old_name", w.entityName),
						zap.String("new_name", userName))
					w.entityName = userName
				}
			}
		}
		// might just need to init client ecc key here.
		return UsshKey, w.generateSessionKeys()
	}
	w.log.Info("unable to read ussh user cert, asking wonkad for keys.", zap.Error(usshErr))

	wonkadErr := w.askWonkadForKeys()
	if wonkadErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"),
			zap.String("source", "wonkad"),
		)

		go w.refreshWonkaCert(ctx, certRefreshPeriod)
		return CertificateKey, nil
	}
	w.log.Info("unable to get key from wonkad", zap.Error(wonkadErr))

	w.metrics.Counter("key-invalid").Inc(1)

	return KeyInvalid, fmt.Errorf("load keys failed: (env: %v), (cfg: %v), (ussh: %v), (wonkad: %v)", envErr, cfgErr, usshErr, wonkadErr)
}

func (w *uberWonka) Admin(ctx context.Context, req AdminRequest) (err error) {
	m := w.metrics.Tagged(map[string]string{"endpoint": "admin"})
	stopWatch := m.Timer("time").Start()
	defer stopWatch.Stop()
	m.Counter("call").Inc(1)
	defer func() {
		name := "success"
		if err != nil {
			// TODO(jkline): Differentiate between 400ish client side failures
			// and 500ish server side failures. Currently we don't get back the
			// http response object so there is no firm way to tell.
			name = "failure"
		}
		m.Counter(name).Inc(1)
	}()

	if w.verifyOnly {
		return errVerifyOnly
	}

	if w.ussh == nil {
		return errors.New("no ussh cert")
	}

	req.Ctime = time.Now().Unix()
	req.Ussh = string(ssh.MarshalAuthorizedKey(w.ussh))
	toSign, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marsalling admin request: %v", err)
	}

	sig, err := w.sshSignMessage(toSign)
	if err != nil {
		return fmt.Errorf("signing ssh message: %v", err)
	}
	req.Signature = base64.StdEncoding.EncodeToString(sig.Blob)
	req.SignatureFormat = sig.Format

	switch req.Action {
	case DeleteEntity:
		w.log.Debug("request to delete",
			zap.Any("entity_to_delete", req.EntityName),
		)

		var resp GenericResponse
		if err := w.httpRequest(ctx, adminEndpoint, req, &resp); err != nil {
			if resp.Result != "" {
				err = fmt.Errorf("%s", resp.Result)
			}
			w.log.Error("https request error",
				zap.Error(err),
				zap.Any("action", req.Action),
				zap.Any("entity_to_delete", req.EntityName),
			)

			return err
		}
		w.log.Debug("response", zap.Any("response", resp.Result))

	default:
		return fmt.Errorf("invalid admin action: %s", req.Action)
	}

	return nil
}

// initClientECC sets up the client ecdsa key based on the client's rsa private key.
func (w *uberWonka) initClientECC() error {
	k, ok := w.clientKey.(*rsa.PrivateKey)
	if !ok {
		return errors.New("not an rsa private key")
	}

	w.clientKeysMu.Lock()
	defer w.clientKeysMu.Unlock()
	w.clientECC = ECCFromRSA(k)

	return nil
}

// initLogAndMetrics initializes the logger and metrics scope.
// Values from config object will be used, if given. Otherwise, default values
// will be used so we don't force extra configuration on consumers.
// Either way, logger will be namespaced to wonka and the metrics will be
// scoped to component:wonka and the given entity name.
func (w *uberWonka) initLogAndMetrics(cfg Config) error {
	ms := cfg.Metrics
	l := cfg.Logger
	if ms == nil {
		ms = tally.NoopScope
	}
	if l == nil {
		l = zap.L()
	}
	w.log = l.With(zap.Namespace("wonka"), zap.String("entity", w.entityName))
	w.metrics = ms.Tagged(map[string]string{
		"component": "wonka",
		"entity":    w.entityName,
		"version":   Version,
	})
	return nil
}

// initTracerAndHTTPClient initializes a tracer and an http client with an
// appropriate tracing filter. The tracer from config object will be used, if
// given. Otherwise opentracing.GlobalTracer will be used.
func (w *uberWonka) initTracerAndHTTPClient(cfg Config) {
	w.tracer = cfg.Tracer
	if w.tracer == nil {
		w.tracer = opentracing.GlobalTracer()
	}
	tracer := xhttp.Tracer{Tracer: w.tracer}
	clientFilter := xhttp.ClientFilterFunc(tracer.TracedClient)
	w.httpClient = &xhttp.Client{
		// Client timeout is max upper bound. Set a lower timeout using ctx.
		Client: http.Client{Timeout: 10 * time.Second},
		// Explicitly set filter to avoi the default client filter with the
		// default global tracer.
		Filter: clientFilter,
	}
}

// EntityName returns the EntityName.
func (w *uberWonka) EntityName() string {
	return w.entityName
}

// Ping tests that we can connect to the wonkamaster
func (w *uberWonka) Ping(ctx context.Context) error {
	var in interface{}
	var out interface{}
	return w.httpRequest(ctx, healthEndpoint, in, out)
}

// Update updates an existing enrollment.
func (w *uberWonka) Update(ctx context.Context, location string, claims []string) (*Entity, error) {
	return w.Enroll(ctx, location, claims)
}

// Enroll enrolls the currently entity with the wonkamaster.
// Allows you to enroll yourself.
func (w *uberWonka) Enroll(ctx context.Context, location string, claims []string) (*Entity, error) {
	if w.ussh != nil {
		return nil, errors.New("unable to enroll in userRequest mode")
	}

	if w.readCertificate() != nil {
		// don't allow enrolling with a certificate since a certifiate implies non-enrollment.
		w.log.Info("ignoring request to enroll/update since we have a certificate")
		return nil, nil
	}

	entity := &Entity{
		EntityName: w.entityName,
		Location:   location,
		Requires:   strings.Join(claims, ","),
		Ctime:      int(time.Now().Unix()),
		SigType:    SHA256,
		Version:    EntityVersion,
	}

	key, ok := w.clientKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an rsa key")
	}

	b, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		w.log.Error("x509 marshalling",
			zap.String("enrollee_entity", entity.EntityName),
			zap.Error(err),
		)

		return nil, err
	}
	entity.PublicKey = base64.StdEncoding.EncodeToString(b)

	w.log.Debug("setting entity ecc public key")
	pubKey := w.readECCKey().PublicKey
	entity.ECCPublicKey = KeyToCompressed(pubKey.X, pubKey.Y)

	toSign := fmt.Sprintf("%s<%d>%s", entity.EntityName, entity.Ctime, entity.PublicKey)
	entity.EntitySignature, err = w.signMessage(toSign, crypto.SHA256)
	if err != nil {
		w.log.Error("adding entity signature", zap.Error(err))
		return nil, err
	}

	return w.EnrollEntity(ctx, entity)
}

// Enroll enrolls the given entity with the wonkamaster.
// Allows you to enroll someone else.
func (w *uberWonka) EnrollEntity(ctx context.Context, e *Entity) (_ *Entity, err error) {
	m := w.metrics.Tagged(map[string]string{"endpoint": "enroll"})
	stopWatch := m.Timer("time").Start()
	defer stopWatch.Stop()
	m.Counter("call").Inc(1)
	defer func() {
		name := "success"
		if err != nil {
			// TODO(jkline): Differentiate between 400ish client side failures
			// and 500ish server side failures. Currently we don't get back the
			// http response object so there is no firm way to tell.
			name = "failure"
		}
		m.Counter(name).Inc(1)
	}()

	if w.verifyOnly {
		w.log.Error("cannot enroll entity in verifyonly mode",
			zap.String("enrollee_entity", e.EntityName),
			zap.Bool("verifyOnly", w.verifyOnly),
		)

		return nil, errVerifyOnly
	}

	enrollReq := EnrollRequest{Entity: e}
	// try to include an engineering claim with this enroll request.
	cr := ClaimRequest{
		Version:     SignEverythingVersion,
		EntityName:  w.entityName,
		Claim:       EnrollerGroup,
		Ctime:       time.Now().Unix(),
		Etime:       time.Now().Add(time.Minute).Unix(),
		Destination: "wonkamaster",
		SigType:     SHA256,
	}

	w.log.Debug("fetch claim before enrolling",
		zap.String("entity", cr.EntityName),
		zap.Bool("has_ussh", w.ussh != nil),
	)

	claim, err := w.doRequestClaim(ctx, cr)
	if err == nil {
		enrollReq.Claim = claim
	} else {
		w.log.Debug("error requesting claim", zap.Any("claim", EnrollerGroup),
			zap.Error(err))
	}

	var resp EnrollResponse
	err = w.httpRequest(ctx, enrollEndpoint, enrollReq, &resp)
	if err != nil {
		return nil, fmt.Errorf("error from %s: %v", enrollEndpoint, err)
	}
	return &resp.Entity, nil
}

// Lookup returns the wonka entity associated with the entity, if it exists.
func (w *uberWonka) Lookup(ctx context.Context, entity string) (_ *Entity, err error) {
	m := w.metrics.Tagged(map[string]string{"endpoint": "lookup"})
	stopWatch := m.Timer("time").Start()
	defer stopWatch.Stop()
	m.Counter("call").Inc(1)
	defer func() {
		name := "success"
		if err != nil {
			// TODO(jkline): Differentiate between 400ish client side failures
			// and 500ish server side failures. Currently we don't get back the
			// http response object so there is no firm way to tell.
			name = "failure"
		}
		m.Counter(name).Inc(1)
	}()
	w.log.Debug("lookup", zap.Any("requested_entity", entity))

	if w.verifyOnly {
		w.log.Error("lookup requested in verify only mode",
			zap.Any("requested_entity", entity),
			zap.Error(errVerifyOnly),
		)

		return nil, errVerifyOnly
	}

	l := LookupRequest{
		Version:         SignEverythingVersion,
		EntityName:      w.entityName,
		RequestedEntity: entity,
		Ctime:           int(time.Now().Unix()),
		SigType:         SHA256,
	}

	toSign, err := json.Marshal(l)
	if err != nil {
		w.log.Error("marshalling lookup request", zap.Error(err))
		return nil, err
	}

	sig, err := wonkacrypter.New().Sign(toSign, w.readECCKey())
	if err != nil {
		w.log.Error("signing lookup request", zap.Error(err))
		return nil, err
	}
	l.Signature = base64.StdEncoding.EncodeToString(sig)

	if w.ussh != nil {
		l.USSHCertificate = string(ssh.MarshalAuthorizedKey(w.ussh))

		toSign, err := json.Marshal(l)
		if err != nil {
			w.log.Error("marshalling lookup request for ussh signing", zap.Error(err))
			return nil, err
		}

		sig, err := w.sshSignMessage(toSign)
		if err != nil {
			w.log.Error("ssh signing message", zap.Error(err))
			return nil, err
		}

		l.USSHSignatureType = sig.Format
		l.USSHSignature = base64.StdEncoding.EncodeToString(sig.Blob)
	}

	var resp LookupResponse
	err = w.httpRequest(ctx, lookupEndpoint, l, &resp)
	if err != nil {
		return nil, fmt.Errorf("lookup httpRequest: %v", err)
	}

	if strings.HasPrefix(resp.Result, "REJECTED") {
		return nil, fmt.Errorf("%s", resp.Result)
	}

	e := resp.Entity
	entityBytes, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("error marshalling entity for signature verification: %v", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil {
		return nil, fmt.Errorf("base64 error: %v", err)
	}

	if ok := wonkacrypter.VerifyAny(entityBytes, sigBytes, WonkaMasterPublicKeys); ok {
		return &e, nil
	}

	return nil, fmt.Errorf("lookup signature does not verify")
}

// LastError is here for libwonka compatibility.
func (w *uberWonka) LastError() string { return "" }

// Certificate returns the certificate if one exists, otherwise it returns nil.
func (w *uberWonka) Certificate() *Certificate {
	return w.readCertificate()
}

// NewEntityCrypter returns a new instance of wonkacrypter.EntityCrypter that
// utilizes this instance of Wonka.
//
// Will return a nil instance if an EntityCrypter cannot be instantiated.
func (w *uberWonka) NewEntityCrypter() wonkacrypter.EntityCrypter {
	return wonkacrypter.NewEntityCrypter(w.readECCKey())
}

// setWonkaURL sets the wonkaURL which is used for all web requests.
func (w *uberWonka) setWonkaURL(ctx context.Context, wonkaMasterURL string) {
	// If Wonkamaster URL is specified in the config
	if wonkaMasterURL != "" {
		w.wonkaURL = wonkaMasterURL
		return
	}

	// If Wonkamaster host & port are specified in environment vars
	h := os.Getenv("WONKA_MASTER_HOST")
	p := os.Getenv("WONKA_MASTER_PORT")
	if h != "" && p != "" {
		w.wonkaURL = fmt.Sprintf("http://%s:%s", h, p)
		return
	}

	// Discover the wonkamaster host & port
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	wonkaURL := ""
	var once sync.Once

	go func() {
		in := struct{}{}
		out := struct{}{}
		url := fmt.Sprintf("%s%s", prodURL, healthEndpoint)
		if err := w.httpRequestWithURL(ctx, url, in, &out); err == nil {
			once.Do(func() {
				wonkaURL = prodURL
				close(done)
			})
		}
	}()

	go func() {
		in := struct{}{}
		out := struct{}{}
		url := fmt.Sprintf("%s%s", localhostURL, healthEndpoint)
		if err := w.httpRequestWithURL(ctx, url, in, &out); err == nil {
			once.Do(func() {
				wonkaURL = localhostURL
				close(done)
			})
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		// timed out or cancel called explicitly
		wonkaURL = externalURL
	}

	w.wonkaURL = wonkaURL
	return
}

// generateSessionKeys will set ClientRSA to a newly generated rsa key.
func (w *uberWonka) generateSessionKeys() error {
	var err error
	w.clientKey, err = rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("error generating rsa session key: %v", err)
	}

	return w.initClientECC()
}

// signMessage signs toSign with the client's private key using the given
// hashing algorithm.
// TODO(pmoody): this should probably be signing with the client ecdsa key
func (w *uberWonka) signMessage(toSign string, hasher crypto.Hash) (string, error) {
	key, ok := w.clientKey.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("client key is not an rsa private key")
	}

	h := hasher.New()
	h.Write([]byte(toSign))

	b, err := key.Sign(rand.Reader, h.Sum(nil), hasher)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// loadPrivateKey returns the rsa.PrivateKey associated with the given pem or
// filename.
func (w *uberWonka) loadPrivateKey(keyPath string) (*rsa.PrivateKey, error) {
	if keyPath == "" {
		keyPath = "./config/secrets.yaml"
	}
	w.log.Debug("loading private key", zap.Any("path", keyPath))

	b := []byte(keyPath)
	var privBytes []byte
	var err error

	privBytes, err = ioutil.ReadFile(keyPath)
	if err == nil {
		b = privBytes
	}

	if strings.EqualFold(filepath.Ext(keyPath), ".yaml") {
		// it's a yaml file
		privBytes, err = w.parseLangleyYAML(b)
		if err != nil {
			w.log.Debug("parsing key from yaml file",
				zap.Error(err),
				zap.Any("file", keyPath),
			)

			return nil, err
		}
	} else {
		// otherwise it might be a pem file/string
		p, _ := pem.Decode(b)
		if p == nil {
			w.log.Debug("error decoding pem", zap.Any("path", keyPath))
			return nil, fmt.Errorf("no pem in private key file")
		}
		privBytes = p.Bytes
	}

	k, err := x509.ParsePKCS1PrivateKey(privBytes)
	if err != nil {
		w.log.Info("x509 parsing failed",
			zap.Error(err),
			zap.Any("path", keyPath),
		)

		return nil, fmt.Errorf("error parsing private key pem bytes: %v", err)
	}

	return k, nil
}

func (w uberWonka) parseLangleyYAML(yamlBytes []byte) ([]byte, error) {
	var b SecretsYAML
	if err := yaml.Unmarshal(yamlBytes, &b); err != nil {
		w.log.Debug("yaml unmarshal error", zap.Error(err))
		return nil, err
	}

	keyStr := strings.TrimPrefix(b.WonkaPrivate, rsaPrivHeader)
	keyStr = strings.TrimSuffix(keyStr, rsaPrivFooter)

	pemBytes, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		w.log.Debug("base64 decode", zap.Error(err))
		return nil, err
	}

	return pemBytes, nil
}

// can these be replaced with atomic ops?
func (w *uberWonka) readECCKey() *ecdsa.PrivateKey {
	w.clientKeysMu.RLock()
	defer w.clientKeysMu.RUnlock()
	k := w.clientECC
	return k
}

func (w *uberWonka) readCertificate() *Certificate {
	w.clientKeysMu.RLock()
	defer w.clientKeysMu.RUnlock()
	c := w.certificate
	return c
}

func (w *uberWonka) writeCertAndKey(c *Certificate, k *ecdsa.PrivateKey) {
	w.clientKeysMu.Lock()
	w.certificate = c
	w.clientECC = k
	w.clientKeysMu.Unlock()

	w.log.Info("stored new certificate",
		zap.Any("serial", c.Serial),
		zap.Any("entity", c.EntityName),
		zap.Any("validAfter", time.Unix(int64(c.ValidAfter), 0)),
		zap.Any("validBefore", time.Unix(int64(c.ValidBefore), 0)),
		zap.Any("tags", c.Tags))
}
