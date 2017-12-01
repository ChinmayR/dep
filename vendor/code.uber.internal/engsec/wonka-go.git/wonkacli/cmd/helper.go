package cmd

import (
	"bufio"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/urfave/cli"
	"go.uber.org/zap"
)

// WonkaClientType represents the intended purpose of a Wonka client
type WonkaClientType int

const (
	// DefaultClient specifies a generic Wonka client
	DefaultClient WonkaClientType = iota
	// EnrollmentClient is intended to perform entity enrollments
	EnrollmentClient
	// EnrollmentClientGenerateKeys is an enrollment client without
	// pre-generated keys.
	EnrollmentClientGenerateKeys
	// DeletionClient is intended to perform an administrative action
	DeletionClient
	// ImpersonationClient specifies a client for impersonation purposes
	ImpersonationClient
	// TaskLauncherClient specifies a client for work on our mesos
	//	cluster.  Specifically it creates a wonka client with an
	//	ssh-agent with the ssh host key loaded.  If you use this client,
	//	you probably need to be running as root.
	TaskLauncherClient
)

// CLIContext wraps a *cli.Context in a sane way
type CLIContext interface {
	Metadata() map[string]interface{}
	Args() cli.Args
	Context() context.Context
	GlobalBool(string) bool
	GlobalString(string) string
	Duration(string) time.Duration
	String(string) string
	StringOrFirstArg(string) string
	StringSlice(string) []string
	Bool(string) bool
	Writer() io.Writer
	Command() cli.Command
	WonkaConfig() wonka.Config
	NewKeyHelper() keyhelper.KeyHelper
	NewWonkaClientFromConfig(wonka.Config) (wonka.Wonka, error)
	NewWonkaClient(WonkaClientType) (wonka.Wonka, error)
}

type cliWrapper struct {
	inner *cli.Context
}

func (c cliWrapper) Metadata() map[string]interface{} {
	return c.inner.App.Metadata
}

func (c cliWrapper) Args() cli.Args {
	return c.inner.Args()
}

func (c cliWrapper) Context() context.Context {
	return c.Metadata()["ctx"].(context.Context)
}

func (c cliWrapper) Duration(s string) time.Duration {
	return c.inner.Duration(s)
}

func (c cliWrapper) GlobalBool(s string) bool {
	return c.inner.GlobalBool(s)
}

func (c cliWrapper) GlobalString(s string) string {
	return c.inner.GlobalString(s)
}

func (c cliWrapper) String(s string) string {
	return c.inner.String(s)
}

func (c cliWrapper) StringOrFirstArg(s string) string {
	value := c.String(s)

	if value == "" {
		value = c.Args().Get(0)
	}

	return value
}

func (c cliWrapper) StringSlice(s string) []string {
	return c.inner.StringSlice(s)
}

func (c cliWrapper) Bool(s string) bool {
	return c.inner.Bool(s)
}

func (c cliWrapper) Writer() io.Writer {
	return c.inner.App.Writer
}

func (c cliWrapper) Command() cli.Command {
	return c.inner.Command
}

func (c cliWrapper) WonkaConfig() wonka.Config {
	return c.Metadata()["config"].(wonka.Config)
}

// NewKeyHelper returns a KeyHelper object for generating crypto keys, and can
// be mocked for testing.
func (c cliWrapper) NewKeyHelper() keyhelper.KeyHelper {
	return keyhelper.New()
}

// Creates a new Wonka client from the provided config (with error logging)
func (c cliWrapper) NewWonkaClientFromConfig(cfg wonka.Config) (wonka.Wonka, error) {
	// wonkacli ends up creating several new clients when it boots up. In general, you
	// should not re-set these variables.
	oldCert := os.Getenv("WONKA_CLIENT_CERT")
	oldKey := os.Getenv("WONKA_CLIENT_KEY")
	defer func() {
		if oldCert != "" {
			os.Setenv("WONKA_CLIENT_CERT", oldCert)
		}
		if oldKey != "" {
			os.Setenv("WONKA_CLIENT_KEY", oldKey)
		}
	}()

	// set the wonkamaster URL, if provided
	url := c.GlobalString("wonkamasterurl")
	if url != "" {
		zap.L().Info("setting wonkamaster URL", zap.String("wonkamasterurl", url))
		cfg.WonkaMasterURL = url
	}

	w, err := wonka.InitWithContext(c.Context(), cfg)
	if err != nil {
		zap.L().Error("error getting Wonka client", zap.Error(err))
		return nil, err
	}

	return w, nil
}

func (c cliWrapper) NewWonkaClient(t WonkaClientType) (wonka.Wonka, error) {
	cfg := c.WonkaConfig()

	switch t {
	case TaskLauncherClient:
		a, name, err := usshHostAgent()
		if err != nil {
			return nil, fmt.Errorf("error creating an agent with the host key: %v", err)
		}
		cfg.EntityName = name
		cfg.Agent = a
	case EnrollmentClient:
		cfg.EntityName = c.StringOrFirstArg("entity")
	case EnrollmentClientGenerateKeys:
		priv, _, err := generateKeys()
		if err != nil {
			return nil, err
		}
		cfg.PrivateKeyPath = string(priv)
	case ImpersonationClient:
		cfg.EntityName = c.GlobalString("self")
	}

	return c.NewWonkaClientFromConfig(cfg)
}

func generateKeys() ([]byte, []byte, error) {
	k, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, cli.NewExitError("error generating keys", 1)
	}

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	privPem := pem.EncodeToMemory(privBlock)

	zap.L().Info("writing wonka_private")
	if err := ioutil.WriteFile("wonka_private", privPem, 0444); err != nil {
		return nil, nil, cli.NewExitError(err.Error(), 1)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		return nil, nil, cli.NewExitError(err.Error(), 1)
	}

	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	pubPem := pem.EncodeToMemory(pubBlock)

	zap.L().Info("writing wonka_public")
	if err := ioutil.WriteFile("wonka_public", pubPem, 0444); err != nil {
		return nil, nil, cli.NewExitError(err.Error(), 1)
	}

	return privPem, pubPem, nil
}

func usshHostAgent() (agent.Agent, string, error) {
	cert, privKey, err := usshHostCert("/etc/ssh/sshd_config")
	if err != nil {
		return nil, "", fmt.Errorf("error loading host key: %v", err)
	}

	a := agent.NewKeyring()
	if err := a.Add(agent.AddedKey{PrivateKey: privKey, Certificate: cert}); err != nil {
		return nil, "", fmt.Errorf("error adding keys to agent: %v", err)
	}

	return a, cert.ValidPrincipals[0], nil
}

func usshHostCert(config string) (*ssh.Certificate, crypto.PrivateKey, error) {
	f, err := os.Open(config)
	if err != nil {
		return nil, nil, fmt.Errorf("opening config: %v", err)
	}

	certPath := ""
	var privKeys []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		if strings.HasPrefix(l, "HostCertificate ") {
			parts := strings.Split(l, " ")
			if len(parts) == 2 {
				certPath = parts[1]
			}
		}

		if strings.HasPrefix(l, "HostKey ") {
			parts := strings.Split(l, " ")
			if len(parts) == 2 {
				privKeys = append(privKeys, parts[1])
			}
		}
	}

	if certPath == "" {
		return nil, nil, errors.New("no cert path")
	}

	cert, err := certFromPath(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading certificate: %v", err)
	}

	privKey, err := privKeyFromPath(cert, privKeys)
	if err != nil {
		return nil, nil, fmt.Errorf("getting host key: %v", err)
	}

	return cert, privKey, nil
}

func certFromPath(path string) (*ssh.Certificate, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening certificate: %v", err)
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(b)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %v", err)
	}

	c, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, errors.New("pubkey not a certificate")
	}

	return c, nil
}

func privKeyFromPath(c *ssh.Certificate, privKeys []string) (crypto.PrivateKey, error) {
	keyType := ""

	switch c.Key.Type() {
	case ssh.KeyAlgoRSA:
		keyType = "rsa"
	case ssh.KeyAlgoDSA:
		keyType = "dsa"
	case ssh.KeyAlgoED25519:
		keyType = "ed25519"
	case ssh.KeyAlgoECDSA256:
		keyType = "ecdsa"
	default:
		return nil, errors.New("invalid key type")
	}

	keyLoc := ""
	for _, pk := range privKeys {
		if strings.HasSuffix(pk, fmt.Sprintf("ssh_host_%s_key", keyType)) {
			keyLoc = pk
			break
		}
	}

	b, err := ioutil.ReadFile(keyLoc)
	if err != nil {
		return nil, fmt.Errorf("opening private key: %v", err)
	}

	k, err := ssh.ParseRawPrivateKey(b)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key: %v", err)
	}

	return k, nil
}
