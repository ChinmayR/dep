package wonka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var cachePeriod = 24 * time.Hour

// Encrypt encrypts data for a particular entity.
// Deprecated
func (w *uberWonka) Encrypt(ctx context.Context, plainText []byte, entity string) ([]byte, error) {
	remotePublic, err := w.entityPubKey(ctx, entity)
	if err != nil {
		w.log.Error("error loading pubkey",
			zap.Any("requested_entity", entity),
			zap.Error(err),
		)

		return nil, err
	}

	return wonkacrypter.New().Encrypt(plainText, w.readECCKey(), remotePublic)
}

// Decrypt decrypts data encrypted by a particular entity
// Deprecated
func (w *uberWonka) Decrypt(ctx context.Context, cipherText []byte, entity string) ([]byte, error) {
	remotePublic, err := w.entityPubKey(ctx, entity)
	if err != nil {
		w.log.Error("error loading pubkey",
			zap.Any("requested_entity", entity),
			zap.Error(err),
		)

		return nil, err
	}

	return wonkacrypter.New().Decrypt(cipherText, w.readECCKey(), remotePublic)

}

// Sign signs data such that the signature can be verified has having come from us.
func (w *uberWonka) Sign(data []byte) ([]byte, error) {
	return wonkacrypter.New().Sign(data, w.readECCKey())
}

// Verify verifies that the given data was signed by the named entity.
func (w *uberWonka) Verify(ctx context.Context, data, sig []byte, entity string) bool {
	remotePublic, err := w.entityPubKey(ctx, entity)
	if err != nil {
		w.log.Debug("error loading key from cache, trying online lookup",
			zap.Any("requested_entity", entity),
			zap.Error(err),
		)

		return false
	}

	return wonkacrypter.New().Verify(data, sig, remotePublic)
}

func (w *uberWonka) entityPubKey(ctx context.Context, entity string) (*ecdsa.PublicKey, error) {
	pubKey, err := w.loadPubKeyFromCache(entity)
	if err != nil {
		w.log.Debug("entity key not in cache",
			zap.Any("requested_entity", entity),
			zap.Error(err),
		)

		pubKey, err = w.lookupRemoteKey(ctx, entity)
		if err != nil {
			w.log.Error("error performing online lookup",
				zap.Any("requested_entity", entity),
				zap.Error(err),
			)

			return nil, err
		}
		go w.saveKey(pubKey, entity)
	}

	return pubKey, err
}

func (w *uberWonka) lookupRemoteKey(ctx context.Context, entity string) (*ecdsa.PublicKey, error) {
	e, err := w.Lookup(ctx, entity)
	if err != nil {
		w.log.Error("error looking up entity",
			zap.Error(err),
			zap.Any("requested_entity", entity),
		)

		return nil, err
	}

	eccPub, err := KeyFromCompressed(e.ECCPublicKey)
	if err != nil {
		w.log.Error("error parsing key",
			zap.Error(err),
			zap.Any("requested_entity", entity),
		)

		return nil, err
	}

	w.log.Debug("remote key",
		zap.Any("key", KeyHash(eccPub)),
		zap.Any("requested_entity", entity),
	)

	return eccPub, nil
}

func (w *uberWonka) saveKey(ecPub *ecdsa.PublicKey, entity string) {
	w.cachedKeysMu.Lock()
	defer w.cachedKeysMu.Unlock()
	w.cachedKeys[entity] = entityKey{time.Now(), ecPub}
}

func (w *uberWonka) loadPubKeyFromCache(entity string) (*ecdsa.PublicKey, error) {
	w.cachedKeysMu.RLock()
	defer w.cachedKeysMu.RUnlock()
	if k, ok := w.cachedKeys[entity]; ok {
		if k.ctime.After(time.Now().Add(-cachePeriod)) {
			return k.key, nil
		}
	}
	return nil, errors.New("no such key")
}

// KeyHash will return the SHA256 hash of a given key.
func KeyHash(key interface{}) string {
	var b []byte
	var err error

	switch k := key.(type) {
	case *rsa.PrivateKey:
		b = x509.MarshalPKCS1PrivateKey(k)
	case *rsa.PublicKey:
		b, err = x509.MarshalPKIXPublicKey(k)
	case *ecdsa.PrivateKey:
		b, err = x509.MarshalECPrivateKey(k)
	case *ecdsa.PublicKey:
		b = elliptic.Marshal(elliptic.P256(), k.X, k.Y)
	case *ssh.PublicKey:
		b = ssh.PublicKey(*k).Marshal()
	default:
		err = fmt.Errorf("unknown key type: %v", reflect.TypeOf(k).String())
	}

	if err != nil {
		// TODO(abg): Inject logger here
		zap.L().Error("decoding", zap.Error(err))
		return ""
	}

	if len(b) == 0 {
		return ""
	}

	return sha256Hash(b)
}

// sha256Hash prints the SHA256 sum of a given set of bytes.
func sha256Hash(b []byte) string {
	h := sha256.New()
	h.Write(b)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
