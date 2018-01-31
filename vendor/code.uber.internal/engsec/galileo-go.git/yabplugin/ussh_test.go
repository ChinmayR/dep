package yabplugin

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"golang.org/x/crypto/ssh"

	"github.com/stretchr/testify/require"
)

func TestGetUIDFromUsshCertificateNoAgentError(t *testing.T) {
	defer setEnvWithCleanup("SSH_AUTH_SOCK", "")()
	err := isUsshCertValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't connect to ssh agent")
}

func TestGetUIDFromUsshCertificateInvalidCAFile(t *testing.T) {
	wonkatestdata.WithUSSHAgent("", func(agentPath string, caKey ssh.PublicKey) {
		defer setEnvWithCleanup("SSH_AUTH_SOCK", agentPath)()
		err := isUsshCertValid()
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't find cert a ussh cert")
	})
}

func TestGetUIDFromUsshCertificate(t *testing.T) {
	wonkatestdata.WithUSSHAgent("foo", func(agentPath string, caKey ssh.PublicKey) {
		wonkatestdata.WithTempDir(func(dir string) {
			defer setEnvWithCleanup("SSH_AUTH_SOCK", agentPath)()
			caFile := path.Join(dir, "trusted_user_ca")
			err := ioutil.WriteFile(caFile, ssh.MarshalAuthorizedKey(caKey), os.ModePerm)
			if err != nil {
				panic(err)
			}
			defer setUserCAWithCleanup(caFile)()
			err = isUsshCertValid()
			require.NoError(t, err)
		})
	})
}
