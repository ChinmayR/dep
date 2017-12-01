package keyhelper_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"path"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRSAFromFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		testdata.WithTempDir(func(dir string) {
			pemFile := path.Join(dir, "a.private.pem")
			privKey := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
			err := testdata.WritePrivateKey(privKey, pemFile)
			require.NoError(t, err, "error writing rsa private key %v", err)

			actual, err := keyhelper.New().RSAFromFile(pemFile)
			require.NoError(t, err, "error reading rsa private key %v", err)
			assert.Equal(t, privKey, actual, "private key doesn't match")
		})
	})

	t.Run("file contains invalid key", func(t *testing.T) {
		testdata.WithTempDir(func(dir string) {
			pemFile := path.Join(dir, "b.private.pem")
			err := ioutil.WriteFile(pemFile, []byte("Happiness is the key to success."), 0440)
			require.NoError(t, err, "error writing file %v", err)

			_, err = keyhelper.New().RSAFromFile(pemFile)
			require.Error(t, err, "expected error reading invalid pem")
			assert.Contains(t, err.Error(), "failed to decode pem")
		})
	})

	t.Run("no such file", func(t *testing.T) {
		testdata.WithTempDir(func(dir string) {
			pemFile := path.Join(dir, "c.private.pem")
			// Don't write the file. Attempt to read file that does not exist.
			_, err := keyhelper.New().RSAFromFile(pemFile)
			require.Error(t, err, "expected error reading nonexistent file")
			assert.Contains(t, err.Error(), "no such file")
		})
	})
}

func TestRSAAndECC(t *testing.T) {
	wonkatestdata.WithTempDir(func(dir string) {
		keyFile := path.Join(dir, "key.pem")

		_, _, _, err := keyhelper.New().RSAAndECC(keyFile)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such file or directory")

		k, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		pemBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		}

		err = ioutil.WriteFile(keyFile, pem.EncodeToMemory(pemBlock), 0666)
		require.NoError(t, err)

		rsaKey, _, eccPub, err := keyhelper.New().RSAAndECC(keyFile)
		require.NoError(t, err)

		rsaKeyBytes := x509.MarshalPKCS1PrivateKey(rsaKey)
		kBytes := x509.MarshalPKCS1PrivateKey(k)
		require.True(t, bytes.Equal(rsaKeyBytes, kBytes))

		eccPriv := wonka.ECCFromRSA(k)
		ePub := wonka.KeyToCompressed(eccPriv.PublicKey.X, eccPriv.PublicKey.Y)
		require.Equal(t, ePub, eccPub)
	})
}
