package sshhelper

import (
	"bufio"
	"crypto"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// UsshHostCert returns the public key and ssh certificate configured in the given sshd_config.
func UsshHostCert(log *zap.Logger, config string) (*ssh.Certificate, crypto.PrivateKey, error) {
	log.Debug("opening sshd_config", zap.String("config", config))
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
	if len(privKeys) == 0 {
		return nil, errors.New("no private keys")
	}

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
	for _, k := range privKeys {
		if strings.HasSuffix(k, fmt.Sprintf("ssh_host_%s_key", keyType)) {
			keyLoc = k
			break
		}
	}

	if keyLoc == "" {
		return nil, fmt.Errorf("no host keys of type %q", keyType)
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
