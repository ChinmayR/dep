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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/redswitch"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

const _defaultPrivateKeyPath = "./config/secrets.yaml"

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
	// Deprecated. use wonkacrypter.Sign instead.
	Sign(data []byte) ([]byte, error)

	// Verify verifies that data was signed by private ecc key held
	// by the named entity. If needed, it will contact wonkamaster to get the
	// publickey for the named entity and cache the result.
	// Deprecated. use wonkacrypter.Verify instead.
	Verify(ctx context.Context, data, sig []byte, entity string) bool

	// Encrypt encrypts data for entity.
	// Deprecated. use wonkacrypter.Encrypt instead.
	Encrypt(ctx context.Context, plainText []byte, entity string) ([]byte, error)

	// Decrypt decrypts data encrypted by entity.
	// Deprecated. use wonkacrypter.Decrypt() instead.
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

var _globalCertificateRegistry = newCertificateRegistry()

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
		entityName:             cfg.EntityName,
		cachedKeys:             make(map[string]entityKey, 0),
		cachedKeysMu:           sync.RWMutex{},
		clientKeysMu:           sync.RWMutex{},
		sshAgent:               cfg.Agent,
		implicitClaims:         cfg.ImplicitClaims,
		derelicts:              make(map[string]time.Time),
		derelictsLock:          sync.RWMutex{},
		derelictsRefreshPeriod: internal.DerelictsCheckPeriod,
		derelictsTimer:         time.NewTimer(internal.DerelictsCheckPeriod),
		wonkaURLRequested:      cfg.WonkaMasterURL,
		certRepository:         _globalCertificateRegistry,
		globalDisableRecovery:  make(chan time.Time, 1),
	}

	w.initLogAndMetrics(cfg)
	w.initTracer(cfg)

	if len(cfg.WonkaMasterPublicKeys) > 0 {
		WonkaMasterPublicKeys = cfg.WonkaMasterPublicKeys
		WonkaMasterPublicKey = WonkaMasterPublicKeys[0]
	}

	if err := w.initRedswitchReader(); err != nil {
		return nil, err
	}

	// After this point we may need to contact wonkamaster, so we need
	// to know if we are in a globally disabled state or not.
	lookupCtx, lookupCancel := context.WithTimeout(ctx, time.Second)
	defer lookupCancel()
	disabled := w.globalDisableReader.ForceCheckIsDisabled(lookupCtx)
	if disabled {
		w.log.Warn("wonka is currently disabled")

		// Setup recovery
		go func() {
			<-w.globalDisableRecovery
			w.performGlobalDisableRecovery(context.Background())
		}()
	}

	w.httpRequester = newHTTPRequester(w.tracer, w.log)
	if err := w.httpRequester.SetURL(ctx, w.wonkaURLRequested); err != nil {
		if !disabled {
			return nil, fmt.Errorf("error setting wonkamaster url: %v", err)
		}
		w.log.Warn("failed to set wonkamaster URL but ignoring due to global disable",
			zap.Error(err))
	}

	err := w.loadKeyAndUpgrade(ctx, cfg)
	if err != nil {
		return nil, err
	}

	for idx, k := range WonkaMasterPublicKeys {
		w.log.Debug("server ecc key",
			zap.Int("key number", idx),
			zap.String("compressed", KeyToCompressed(k.X, k.Y)))
	}

	w.log.Debug("wonka client initialized",
		zap.String("url", w.httpRequester.URL()),
		zap.String("version", Version),
	)

	go func() {
		if err := w.updateDerelicts(ctx); err != nil {
			w.log.With(zap.Error(err)).Warn("failed to update derelicts")
		}
	}()

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

	err := w.certRepository.Unregister(w.certRegHandle)
	return err
}

// Close terminates all of the background goroutines.
func Close(w Wonka) error {
	closer, ok := w.(Closer)
	if !ok {
		return errors.New("wonka instance must implement wonka.Closer interface")
	}
	return closer.Close()
}

func (w *uberWonka) loadCertAndKeyFromConfig(ctx context.Context, cfg Config) error {
	clientCert := cfg.WonkaClientCertPath
	clientKey := cfg.WonkaClientKeyPath
	if clientKey == "" || clientCert == "" {
		return errors.New("Client cert and/or client key are not set in wonka.Config")
	}
	return w.loadCertAndKeyFromFiles(ctx, clientCert, clientKey)
}

func (w *uberWonka) loadCertAndKeyFromEnv(ctx context.Context) error {
	clientCert := os.Getenv("WONKA_CLIENT_CERT")
	clientKey := os.Getenv("WONKA_CLIENT_KEY")
	if clientKey == "" || clientCert == "" {
		return errors.New("WONKA_CLIENT_CERT and/or WONKA_CLIENT_KEY not set")
	}
	return w.loadCertAndKeyFromFiles(ctx, clientCert, clientKey)
}

// todo(pmoody): consider removing the key and cert from disk.
func (w *uberWonka) loadCertAndKeyFromFiles(ctx context.Context, clientCert, clientKey string) error {
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

	// if we're enabled and this is a cert granting cert, we try to upgrade right now.
	if !w.IsGloballyDisabled() && IsCertGrantingCert(cert) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return w.upgradeCGCert(ctx)
	}

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
		return fmt.Errorf("error unmarshalling reply from wonkad: %s", b)
	}

	cert, err := UnmarshalCertificate(repl.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	key, err := x509.ParseECPrivateKey(repl.PrivateKey)
	if err != nil {
		return fmt.Errorf("error parsing ec private key: %v", err)
	}

	w.writeCertAndKey(cert, key)

	return nil
}

// loadKeyAndUpgrade searches for clientECC in a variety of ways and upgrades to
// a wonka certificate if needed. Also ensures the wonka cert will be refreshed.
func (w *uberWonka) loadKeyAndUpgrade(ctx context.Context, cfg Config) error {
	keyType, entityType, err := w.loadKey(ctx, cfg)
	if err != nil {
		return err
	}

	disabled := w.IsGloballyDisabled()
	if keyType != CertificateKey {
		err = w.upgradeToWonkaCert(ctx, entityType)
		if err != nil {
			log := w.log.With(zap.Error(err))
			if disabled {
				log.Warn("error upgrading to certificate but ignoring due to global disabled")
				return nil
			}
			log.Error("error upgrading to certificate")
			return err
		}
	}

	w.log.Info("registering the wonka certificate for periodic update")
	certRegEntry := certificateRegistrationRequest{
		cert:      w.certificate,
		key:       w.clientECC,
		requester: w.httpRequester,
		log:       w.log,
	}

	handle, err := w.certRepository.Register(certRegEntry)
	if err != nil {
		return fmt.Errorf("failed to register certificate: %v", err)
	}

	w.certRegHandle = handle
	return nil
}

// loadKey searches for clientECC in a variety of ways.
func (w *uberWonka) loadKey(ctx context.Context, cfg Config) (KeyType, EntityType, error) {
	// first check to see if we have a configured cert and key
	if cfg.WonkaClientCert != nil && cfg.WonkaClientKey != nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"), zap.String("source", "config"))

		w.writeCertAndKey(cfg.WonkaClientCert, cfg.WonkaClientKey)

		return CertificateKey, EntityTypeService, nil
	}

	// next check to see if we have a key and cert in config
	cfgErr := w.loadCertAndKeyFromConfig(ctx, cfg)
	if cfgErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"),
			zap.String("source", "config_path"),
		)

		return CertificateKey, EntityTypeService, nil
	}
	w.log.Info("unable to load ecc private key from config paths, trying to load from env.", zap.Error(cfgErr))

	// next check to see if we have a key and cert in environment
	envErr := w.loadCertAndKeyFromEnv(ctx)
	if envErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"),
			zap.String("source", "env"),
		)

		return CertificateKey, EntityTypeService, nil
	}
	w.log.Info("unable to load ecc private key from env paths, trying to load from other config.", zap.Error(envErr))

	// next check to see if we have a configured old-style rsa pem
	pemCfgErr := errors.New("no configured private key pem specified")
	pathCfgErr := errors.New("no configured private key path specified")
	if cfg.PrivateKeyPath != "" {
		var rsaPriv *rsa.PrivateKey
		rsaPriv, pemCfgErr = w.loadKeyFromPem([]byte(cfg.PrivateKeyPath))
		if pemCfgErr == nil {
			w.log.Info("successfully loaded private key",
				zap.String("type", "static"),
				zap.String("source", "config"),
			)

			w.clientKey = rsaPriv
			w.writeECCKey(ECCFromRSA(rsaPriv))

			return StaticKey, EntityTypeService, nil
		}

		rsaPriv, pathCfgErr = w.loadKeyFromPath(cfg.PrivateKeyPath)
		if pathCfgErr == nil {
			w.log.Info("successfully loaded private key",
				zap.String("type", "file"),
				zap.String("source", "config"),
				zap.String("path", cfg.PrivateKeyPath),
			)

			w.clientKey = rsaPriv
			w.writeECCKey(ECCFromRSA(rsaPriv))

			return FileKey, EntityTypeService, nil
		}
	}

	w.log.Info("unable to load rsa private key from config, trying personnel request.",
		zap.NamedError("pem_error", pemCfgErr),
		zap.NamedError("path_error", pathCfgErr),
		zap.Any("sshAgent_is_nil", w.sshAgent == nil))

	var usshErr error
	w.ussh, usshErr = w.usshUserCert()
	if usshErr == nil {
		eType := EntityTypeHost
		w.log.Info("successfully loaded private key", zap.String("type", "ussh"),
			zap.String("source", "ussh"))
		if w.ussh.CertType == ssh.UserCert {
			eType = EntityTypeUser
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

		return UsshKey, eType, nil
	}

	w.log.Info("unable to read ussh user cert, asking wonkad for keys.", zap.Error(usshErr))

	wonkadErr := w.askWonkadForKeys()
	if wonkadErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "certificate"),
			zap.String("source", "wonkad"),
		)

		return CertificateKey, EntityTypeService, nil
	}
	w.log.Info("unable to get key from wonkad", zap.Error(wonkadErr))

	// Try loading a private key from the default location
	var defaultCfgErr error
	var rsaPriv *rsa.PrivateKey
	rsaPriv, defaultCfgErr = w.loadKeyFromPath(_defaultPrivateKeyPath)
	if defaultCfgErr == nil {
		w.log.Info("successfully loaded private key",
			zap.String("type", "file"),
			zap.String("source", "config"),
			zap.String("path", _defaultPrivateKeyPath),
		)

		w.clientKey = rsaPriv
		w.writeECCKey(ECCFromRSA(rsaPriv))

		return FileKey, EntityTypeService, nil
	}

	w.metrics.Counter("key-invalid").Inc(1)
	return KeyInvalid, EntityTypeInvalid, fmt.Errorf(
		"load keys failed: (cfg: %v), (env: %v), (pem: %v), (path: %v), (ussh: %v), (wonkad: %v), (defaultCfg: %v)",
		cfgErr, envErr, pemCfgErr, pathCfgErr, usshErr, wonkadErr, defaultCfgErr,
	)
}

func (w *uberWonka) host() (string, error) {
	if cert := w.readCertificate(); cert != nil {
		return cert.Host, nil
	}

	if w.ussh != nil && w.ussh.CertType == ssh.HostCert {
		return w.ussh.ValidPrincipals[0], nil
	}

	if w.sshAgent != nil {
		if keys, err := w.sshAgent.List(); err == nil {
			for _, k := range keys {
				pubKey, err := ssh.ParsePublicKey(k.Blob)
				if err != nil {
					continue
				}
				cert, ok := pubKey.(*ssh.Certificate)
				if ok && cert.CertType == ssh.HostCert {
					return cert.ValidPrincipals[0], nil
				}
			}
		}
	}

	return os.Hostname()
}

func (w *uberWonka) upgradeToWonkaCert(ctx context.Context, eType EntityType) error {
	hostname, err := w.host()
	if err != nil {
		return fmt.Errorf("error getting hostname: %v", err)
	}

	// if this is a pre-enrolled you might be tempted to just use the pre-set
	// keypair for the certificate. This would be a mistake because it would mean
	// that every instance of a service would have the same key.
	cert, key, err := NewCertificate(CertEntityName(w.entityName),
		CertEntityType(eType), CertHostname(hostname))
	if err != nil {
		return fmt.Errorf("error generating new certificate: %v", err)
	}

	err = w.CertificateSignRequest(ctx, cert, nil)
	if err != nil {
		return fmt.Errorf("error getting cert signed: %v", err)
	}

	w.writeCertAndKey(cert, key)
	return nil
}

// initRedswitchReader initializes the reader that will be used to query for
// global disabled status.
func (w *uberWonka) initRedswitchReader() error {
	pubKeys := make([]crypto.PublicKey, len(WonkaMasterPublicKeys))
	for i := range WonkaMasterPublicKeys {
		pubKeys[i] = WonkaMasterPublicKeys[i]
	}

	reader, err := redswitch.NewReader(
		redswitch.WithLogger(w.log),
		redswitch.WithMetrics(w.metrics),
		redswitch.WithRecoveryNotification(w.globalDisableRecovery),
		redswitch.WithPublicKeys(pubKeys...),
	)
	if err != nil {
		return err
	}

	creader, ok := reader.(redswitch.CachelessReader)
	if !ok {
		return errors.New("failed to type assert redswitch.Reader to redswitch.CachelessReader")
	}

	w.globalDisableReader = creader
	return err
}

// initLogAndMetrics initializes the logger and metrics scope.
// Values from config object will be used, if given. Otherwise, default values
// will be used so we don't force extra configuration on consumers.
// Either way, logger will be namespaced to wonka and the metrics will be
// scoped to component:wonka and the given entity name.
func (w *uberWonka) initLogAndMetrics(cfg Config) {
	ms := cfg.Metrics
	l := cfg.Logger
	if ms == nil {
		ms = tally.NoopScope
	}
	if l == nil {
		l = zap.L()
	}
	w.log = l.With(
		zap.Namespace("wonka"),
		zap.String("entity", w.entityName),
		zap.String("version", LibraryVersion()),
	)
	w.metrics = ms.Tagged(map[string]string{
		"component": "wonka",
		"entity":    w.entityName,
		// Override host for Wonka's scope in case caller set per-host metrics
		// because Wonka already has large metric cardinality.
		"host": "global",
	})
}

// initTracer initializes a tracer.
//
// The tracer from config object will be used, if
// given. Otherwise opentracing.GlobalTracer will be used.
func (w *uberWonka) initTracer(cfg Config) {
	w.tracer = cfg.Tracer
	if w.tracer == nil {
		w.tracer = opentracing.GlobalTracer()
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
	return w.httpRequester.Do(ctx, healthEndpoint, in, out)
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
	err = w.httpRequester.Do(ctx, enrollEndpoint, enrollReq, &resp)
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

	var certBytes []byte
	cert := w.readCertificate()
	if cert == nil {
		// a log.Warn() might seem excessive, but we should be upgrading everything
		// to a wonkacert so this _is_ odd.
		w.log.Warn("no certificate present",
			zap.Any("requesting_entity", w.entityName),
			zap.Any("requested_entity", entity))
	} else {
		certBytes, err = MarshalCertificate(*cert)
		if err != nil {
			certBytes = nil
			w.log.Warn("error marshalling certifcate", zap.Error(err),
				zap.Any("requesting_entity", w.entityName),
				zap.Any("requested_entity", entity))
		}
	}

	l := LookupRequest{
		Version:         SignEverythingVersion,
		EntityName:      w.entityName,
		RequestedEntity: entity,
		Ctime:           int(time.Now().Unix()),
		Certificate:     certBytes,
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

	var resp LookupResponse
	err = w.httpRequester.Do(ctx, lookupEndpoint, l, &resp)
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

	return nil, errors.New("lookup signature does not verify")
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

// loadKeyFromPem returns the rsa.PrivateKey associated with the given pem.
func (w *uberWonka) loadKeyFromPem(pemBytes []byte) (*rsa.PrivateKey, error) {
	p, _ := pem.Decode(pemBytes)
	if p == nil {
		w.log.Debug("error decoding private key pem")
		return nil, errors.New("invalid pem")
	}

	k, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		w.log.Debug("x509 parsing failed", zap.Error(err))

		return nil, fmt.Errorf("error x509 parsing private key: %v", err)
	}

	return k, nil
}

// loadKeyFromPath returns the rsa.PrivateKey contained in the file with the
// given filename.
func (w *uberWonka) loadKeyFromPath(keyPath string) (*rsa.PrivateKey, error) {
	w.log.Debug("loading private key", zap.Any("path", keyPath))

	var privBytes []byte
	var err error

	privBytes, err = ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading private key from file: %v", err)
	}

	if strings.EqualFold(filepath.Ext(keyPath), ".yaml") {
		// it's a yaml file
		k, err := w.parseLangleyYAML(privBytes)
		if err != nil {
			w.log.Debug("parsing key from yaml file",
				zap.Error(err),
				zap.Any("file", keyPath),
			)

			return nil, err
		}
		return k, nil
	}

	k, err := w.loadKeyFromPem(privBytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key pem from file %s: %v", keyPath, err)
	}

	return k, nil
}

func (w *uberWonka) parseLangleyYAML(yamlBytes []byte) (*rsa.PrivateKey, error) {
	var b SecretsYAML
	if err := yaml.Unmarshal(yamlBytes, &b); err != nil {
		w.log.Debug("yaml unmarshal error", zap.Error(err))
		return nil, err
	}

	keyStr := strings.TrimPrefix(b.WonkaPrivate, rsaPrivHeader)
	keyStr = strings.TrimSuffix(keyStr, rsaPrivFooter)

	// Content of langley yaml cannot be pem.Decoded because it doesn't contain
	// any new lines.
	pemBytes, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding private key from langley yaml: %v", err)
	}

	k, err := x509.ParsePKCS1PrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("error x509 parsing private key from langley yaml: %v", err)
	}

	return k, nil
}

// can these be replaced with atomic ops?
func (w *uberWonka) readECCKey() *ecdsa.PrivateKey {
	w.clientKeysMu.RLock()
	defer w.clientKeysMu.RUnlock()
	k := w.clientECC
	return k
}

func (w *uberWonka) writeECCKey(k *ecdsa.PrivateKey) {
	w.clientKeysMu.Lock()
	w.clientECC = k
	w.clientKeysMu.Unlock()
}

func (w *uberWonka) readCertificate() *Certificate {
	w.clientKeysMu.RLock()
	cert := w.certificate
	w.clientKeysMu.RUnlock()

	if w.certRegHandle == nil {
		return cert
	}

	newCert, key := w.certRegHandle.GetCertificateAndPrivateKey()
	if !cert.equal(newCert) {
		cert = newCert
		w.writeCertAndKey(cert, key)
	}

	return cert
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
