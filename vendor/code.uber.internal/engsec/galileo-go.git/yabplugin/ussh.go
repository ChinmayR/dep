package yabplugin

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var userCA = "/etc/ssh/trusted_user_ca"

func isUsshCertValid() error {
	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("couldn't connect to ssh agent: %v", err)
	}
	cert, err := findUsshCert(agent.NewClient(sock), userCA)
	if err != nil {
		return fmt.Errorf("couldn't find cert a ussh cert: %v", err)
	}
	if len(cert.ValidPrincipals) == 0 {
		return fmt.Errorf("no principals found on ussh cert")
	}
	return nil
}

// TODO(tjulian): this is lifted from c:engsec/ussh - it should be replaced
// when ussh removes go-common as a dependency

// findUsshCert returns the first ussh cert found on the agent. This should be
// the *only* ussh cert on the agent.
func findUsshCert(a agent.Agent, userCA string) (*ssh.Certificate, error) {
	caBytes, err := ioutil.ReadFile(userCA)
	if err != nil {
		return nil, fmt.Errorf("error reading userCA: %v", err)
	}

	caKeys, err := parseUserCA(caBytes)
	if err != nil {
		return nil, err
	}

	keys, err := a.List()
	if err != nil {
		return nil, fmt.Errorf("listing agent: %v", err)
	}

	for _, k := range keys {
		if cert := isUsshAgentKey(k, caKeys); cert != nil {
			return cert, nil
		}
	}
	return nil, fmt.Errorf("no ussh certs found")
}

// parseUserCA reads a trusted_user_ca file and turns it into a slice of ssh PublicKey's.
func parseUserCA(userCABytes []byte) ([]ssh.PublicKey, error) {
	var pubKeys []ssh.PublicKey
	in := userCABytes
	var parseErr error
	for {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			parseErr = err
			continue
		}
		pubKeys = append(pubKeys, pubKey)
		if len(rest) == 0 {
			break
		}
		in = rest
	}
	if parseErr != nil {
		return nil, parseErr
	}
	if len(pubKeys) == 0 {
		return nil, errors.New("no keys found")
	}
	return pubKeys, nil
}

// isUssh returns the ssh certificate if the given key is a ussh cert.
func isUssh(pubKey ssh.PublicKey, userCA []ssh.PublicKey) *ssh.Certificate {
	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil
	}

	for _, caKey := range userCA {
		if bytes.Equal(cert.SignatureKey.Marshal(), caKey.Marshal()) {
			return cert
		}
	}
	return nil
}

// isUsshAgentKey returns true if the given ssh-agent key is a ussh cert.
func isUsshAgentKey(key *agent.Key, userCA []ssh.PublicKey) *ssh.Certificate {
	pubKey, err := ssh.ParsePublicKey(key.Marshal())
	if err != nil {
		return nil
	}
	return isUssh(pubKey, userCA)
}
