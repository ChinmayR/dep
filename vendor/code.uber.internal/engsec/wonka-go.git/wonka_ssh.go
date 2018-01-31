package wonka

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	// TODO(pmoody): need to be able to pass this as an argument.
	userCA = "/etc/ssh/trusted_user_ca"
	hostCA = "/etc/ssh/ssh_known_hosts"
)

type usshCheck struct {
	userCA []ssh.PublicKey
	hostCB ssh.HostKeyCallback
}

func (w *uberWonka) connectToSSHAgent() error {
	agentSock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("error connecting to ssh agent: %v", err)
	}
	w.sshAgent = agent.NewClient(agentSock)
	return nil
}

func (w *uberWonka) sshSignMessage(toSign []byte) (*ssh.Signature, error) {
	w.log.Debug("trying to sign message with ssh-agent")
	if w.sshAgent == nil {
		if err := w.connectToSSHAgent(); err != nil {
			return nil, err
		}
	}

	k, err := usshCertWithAgent(w.log, w.sshAgent)
	if err != nil {
		return nil, err
	}

	// ssh signers apply the hash for the given key to the data before signing it.
	sig, err := w.sshAgent.Sign(k, toSign)
	if err != nil {
		return nil, fmt.Errorf("signing data: %v", err)
	}

	return sig, nil
}

//
// TODO(pmoody): everything below here should come from the ussh
// library
//

// usshCert returns the first ussh cert found on the ssh-agent.
// this should be the *only* ussh cert on the agent.
func (w *uberWonka) usshUserCert() (*ssh.Certificate, error) {
	if w.sshAgent == nil {
		if err := w.connectToSSHAgent(); err != nil {
			return nil, err
		}
	}
	return usshCertWithAgent(w.log, w.sshAgent)
}

// usshCertWithAgent returns the first ussh cert found on the ssh-agent.
// this should be the *only* ussh cert on the agent.
func usshCertWithAgent(log *zap.Logger, a agent.Agent) (*ssh.Certificate, error) {
	certCheck := usshCheck{}

	caKeys, err := parseUserCA(log, userCA)
	if err != nil {
		return nil, fmt.Errorf("failed to load user ca file: %v", err)
	}
	certCheck.userCA = caKeys

	// TODO(pmoody): be smarter about dealing with errors here.
	hostCB, err := ParseHostCA(hostCA)
	if err == nil {
		certCheck.hostCB = hostCB
	}

	keys, err := a.List()
	if err != nil {
		return nil, fmt.Errorf("listing agent: %v", err)
	}

	for _, k := range keys {
		if isUsshCert(log, k, certCheck) {
			cert, err := certFromAgentKey(log, k)
			if err != nil {
				// lol wut?
				log.Error("error with cert from agent key", zap.Error(err))
				return nil, fmt.Errorf("we have a cert but we don't: %v", err)
			}
			log.Debug("returning a good cert")
			return cert, nil
		}
	}
	log.Debug("no ussh  certs found")
	return nil, fmt.Errorf("no ussh certs found")
}

// isUsshCert returns true if the given key is a ussh certificate.
func isUsshCert(log *zap.Logger, key *agent.Key, certCheck usshCheck) bool {
	cert, err := certFromAgentKey(log, key)
	if err != nil {
		return false
	}

	switch cert.CertType {
	case ssh.UserCert:
		for _, caKey := range certCheck.userCA {
			if bytes.Equal(cert.SignatureKey.Marshal(), caKey.Marshal()) {
				return true
			}
		}
	case ssh.HostCert:
		if certCheck.hostCB == nil {
			log.Error("host cert but not host key callback")
			return false
		}

		name := fmt.Sprintf("%s:22", cert.ValidPrincipals[0])
		addr := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}
		if err := certCheck.hostCB(name, addr, cert); err != nil {
			log.Error("error validating host cert",
				zap.Error(err),
				zap.Any("signing_key", ssh.FingerprintSHA256(cert.SignatureKey)),
			)

			return false
		}

		log.Debug("valid host cert found")
		return true
	default:
		log.Error("unknown cert type", zap.Any("certtype", cert.CertType))
		return false
	}

	// linting is lame
	return false
}

// certFromAgentKey returns the unmarshalled ssh certificate from
// the given ssh key.
func certFromAgentKey(log *zap.Logger, k *agent.Key) (*ssh.Certificate, error) {
	if k == nil {
		return nil, fmt.Errorf("nil key pointer")
	}

	pubKey, err := ssh.ParsePublicKey(k.Marshal())
	if err != nil {
		return nil, fmt.Errorf("parsing key: %v", err)
	}

	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("not a cert: %s", KeyHash(&pubKey))
	}

	log.Debug("found cert", zap.String("cert", KeyHash(&cert.Key)))
	return cert, nil
}

// parseUserCA parses the user ca file and returns the pubkeys
// used for signing user certs.
func parseUserCA(log *zap.Logger, caFile string) ([]ssh.PublicKey, error) {
	if envCA := os.Getenv("WONKA_USSH_CA"); envCA != "" {
		log.Debug("parsing ca key from environment", zap.Any("key", envCA))
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(envCA))
		if err != nil {
			return nil, err
		}
		return []ssh.PublicKey{pubKey}, nil
	}

	var pubKeys []ssh.PublicKey
	b, err := ioutil.ReadFile(caFile)
	if err != nil {
		return pubKeys, err
	}
	in := b
	for {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, pubKey)
		if len(rest) == 0 {
			break
		}
		in = rest
	}
	return pubKeys, nil
}

// ParseHostCA returns a host key callback based on the system known hosts
// or the known hosts set in the environment (eg. for testing).
func ParseHostCA(caFile string) (ssh.HostKeyCallback, error) {
	envCA := os.Getenv("WONKA_USSH_HOST_CA")
	if caFile == "" || envCA != "" {
		dir, err := ioutil.TempDir("", "wonka")
		if err != nil {
			return nil, fmt.Errorf("error creating temp directory: %v", err)
		}
		defer os.RemoveAll(dir)

		caFile = path.Join(dir, "ssh_known_hosts")
		if err := ioutil.WriteFile(caFile, []byte(envCA), 0400); err != nil {
			return nil, fmt.Errorf("error writing known hosts file: %v", err)
		}
	}

	return knownhosts.New(caFile)
}
