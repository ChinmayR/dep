package wonka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	wonkaMasterEntity   = "wonkamaster"
	wonkaMasterRequires = "AD:engineering"
)

var (
	defaultClaimTTL = 24 * time.Hour
	maxClaimTTL     = 24 * time.Hour

	errVerifyOnly = errors.New("cannot request claims")

	// clockSkew tries to account for, well, clock skew. So
	// ValidAfter + clockSkew > time.Now() and
	// ValidBefore - clockSkew < time.Now()
	clockSkew = time.Minute
)

// MarshalClaim turns a claim into a string for sending on the wire.
func MarshalClaim(c *Claim) (string, error) {
	claimBytes, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshalling claim: %v", err)
	}

	return base64.StdEncoding.EncodeToString(claimBytes), nil
}

// UnmarshalClaim turns a wire-form string claim into a wonka.Claim.
func UnmarshalClaim(c string) (*Claim, error) {
	claimBytes, err := base64.StdEncoding.DecodeString(c)
	if err != nil {
		return nil, fmt.Errorf("base64 unmarshalling claim token: %v", err)
	}

	var claim Claim
	if err := json.Unmarshal(claimBytes, &claim); err != nil {
		return nil, fmt.Errorf("json unmarshalling claim token: %v", err)
	}
	return &claim, nil
}

// Inspect returns true if the claim token is good for a destination listed in
// allowedDests and contain a claim listed in requiredClaims. Returns nil when
// everything is ok with the token. Otherwise returns an error indicating the
// problem.
func (c *Claim) Inspect(allowedDests, requiredClaims []string) error {
	for _, dest := range allowedDests {
		if err := c.Check(dest, requiredClaims); err != nil {
			return err
		}
	}

	return nil
}

// Check checks that a claim token is valid, the destination is dest, and that
// it grants a claim listed in requiredClaims.  Returns nil when everything is
// ok with the token. Otherwise returns an error indicating the problem.
func (c *Claim) Check(dest string, requiredClaims []string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	// now check destination
	if !strings.EqualFold(c.Destination, dest) {
		return fmt.Errorf("claim token destination is not %q", dest)
	}

	grantedClaims := make(map[string]struct{}, 1)
	for _, gc := range c.Claims {
		grantedClaims[strings.ToLower(gc)] = struct{}{}
	}

	for _, ac := range requiredClaims {
		if _, ok := grantedClaims[strings.ToLower(ac)]; ok {
			return nil
		}
	}

	return fmt.Errorf(
		"claim token grants no required claims. requiredClaims=%q; grantedClaims=%q", requiredClaims, c.Claims,
	)
}

// Validate checks that a claim is signed by wonkamaster, and not expired.
// Returns nil when token is valid, otherwise returns an error indicating the
// problem.
func (c *Claim) Validate() error {
	now := time.Now()
	createTime := time.Unix(c.ValidAfter, 0)
	expireTime := time.Unix(c.ValidBefore, 0)

	if now.Add(clockSkew).Before(createTime) {
		return fmt.Errorf("claim token not valid yet: entity=%q, createTime=%d, now=%d",
			c.EntityName, c.ValidAfter, now.Unix())
	}

	if now.Add(-clockSkew).After(expireTime) {
		return fmt.Errorf("claim token expired: entity=%q, expireTime=%d, now=%d",
			c.EntityName, c.ValidBefore, now.Unix())
	}

	claim := Claim{
		ClaimType:   c.ClaimType,
		EntityName:  c.EntityName,
		ValidAfter:  c.ValidAfter,
		ValidBefore: c.ValidBefore,
		Claims:      c.Claims,
		Destination: c.Destination,
	}

	toVerify, err := json.Marshal(claim)
	if err != nil {
		return fmt.Errorf("error marshalling claim for signature check: %v", err)
	}

	if wonkacrypter.VerifyAny(toVerify, c.Signature, WonkaMasterPublicKeys) {
		return nil // valid token
	}

	return errors.New("claim token has invalid signature")
}

// ClaimRequest requests a new wonka claim.
func (w *uberWonka) ClaimRequest(ctx context.Context, claim, destination string) (*Claim, error) {
	return w.ClaimRequestTTL(ctx, claim, destination, defaultClaimTTL)
}

// Execute RPC to request a claim over HTTPS
func (w *uberWonka) doRequestClaim(ctx context.Context, cr ClaimRequest) (ret *Claim, err error) {
	m := w.metrics.Tagged(map[string]string{"endpoint": "claim"})
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
		return nil, errVerifyOnly
	}

	if len(w.implicitClaims) != 0 {
		cr.Claim = fmt.Sprintf("%s,%s", cr.Claim, strings.Join(w.implicitClaims, ","))
	}

	cert := w.readCertificate()
	if cert != nil {
		certBytes, err := MarshalCertificate(*cert)
		if err != nil {
			return nil, err
		}
		cr.Certificate = certBytes
	}

	toSign, err := json.Marshal(cr)
	if err != nil {
		w.log.Error("marshalling claim request for signature", zap.Error(err))
		return nil, fmt.Errorf("marshalling claim request for signature: %v", err)
	}

	eccKey := w.readECCKey()
	sig, err := wonkacrypter.New().Sign(toSign, eccKey)
	if err != nil {
		w.log.Error("signing message", zap.Error(err))
		return nil, fmt.Errorf("signing message: %v", err)
	}
	cr.Signature = base64.StdEncoding.EncodeToString(sig)

	if cert == nil && w.ussh != nil {
		pubKey := eccKey.PublicKey
		cr.SessionPubKey = KeyToCompressed(pubKey.X, pubKey.Y)

		cr.USSHCertificate = string(ssh.MarshalAuthorizedKey(w.ussh))

		// use the cert to sign this message
		toSign, err = json.Marshal(cr)
		if err != nil {
			w.log.Error("marshalling request for ussh signature", zap.Error(err))
			return nil, fmt.Errorf("marshalling request for ussh signature: %v", err)
		}

		sig, err := w.sshSignMessage(toSign)
		if err != nil {
			w.log.Error("ssh signing message", zap.Error(err))
			return nil, fmt.Errorf("ssh signing message: %v", err)
		}

		cr.USSHSignature = base64.StdEncoding.EncodeToString(sig.Blob)
		cr.USSHSignatureType = sig.Format
	}

	var cResp ClaimResponse
	if err := w.httpRequest(ctx, claimEndpoint, cr, &cResp); err != nil {
		w.log.Error("error sending http/s request", zap.Error(err))
		return nil, fmt.Errorf("error from %s: %v", claimEndpoint, err)
	}

	if cResp.Result != "" && cResp.Result != "OK" {
		w.log.Error("wonkamaster returned not ok", zap.Any("result", cResp.Result))
		return nil, fmt.Errorf("error from %s: %v", claimEndpoint, err)
	}

	tok, err := base64.StdEncoding.DecodeString(cResp.Token)
	if err != nil {
		w.log.Error("error decoding token", zap.Error(err))
		return nil, fmt.Errorf("base64 decoding token: %v", err)
	}

	decryptedClaim, err := wonkacrypter.DecryptAny(tok, eccKey, WonkaMasterPublicKeys)
	if err != nil {
		return nil, fmt.Errorf("error decrypting token: %v", err)
	}

	ret = &Claim{}
	if err := json.Unmarshal(decryptedClaim, ret); err != nil {
		w.log.Error("unmarshalling token",
			zap.Error(err),
			zap.Any("claim", cr.Claim),
			zap.Any("destination", cr.Destination),
			zap.Any("impersonated_entity", cr.ImpersonatedEntity),
		)

		return nil, fmt.Errorf("unmarshalling token: %v", err)
	}

	return ret, nil
}

// ClaimRequestTTL will request the given claim with the given scope.
func (w *uberWonka) ClaimRequestTTL(ctx context.Context, claim, dest string, ttl time.Duration) (*Claim, error) {
	cr := ClaimRequest{
		Version:     SignEverythingVersion,
		EntityName:  w.entityName,
		Claim:       claim,
		Destination: dest,
		Ctime:       time.Now().Add(-clockSkew).Unix(),
		Etime:       time.Now().Add(ttl).Unix(),
		SigType:     SHA256,
	}
	return w.doRequestClaim(ctx, cr)
}

func (w *uberWonka) ClaimResolve(ctx context.Context, entityName string) (*Claim, error) {
	return w.ClaimResolveTTL(ctx, entityName, defaultClaimTTL)
}

// ClaimResolveTTL will request a claim for the named entity with the default ttl.
func (w *uberWonka) ClaimResolveTTL(ctx context.Context, entityName string, ttl time.Duration) (*Claim, error) {
	key := w.readECCKey()
	if key == nil {
		return nil, errors.New("wonka ECC private key is nil")
	}

	cert := w.readCertificate()
	var marshalledCert []byte
	if cert != nil {
		// an error here shouldn't matter. If we error, marshalledCert is nil
		// and that field in the ResolveRequest will be empty.
		marshalledCert, _ = MarshalCertificate(*cert)
	}

	pubKey := KeyToCompressed(key.PublicKey.X, key.PublicKey.Y)
	req := ResolveRequest{
		EntityName:      w.entityName,
		RequestedEntity: entityName,
		PublicKey:       pubKey,
		Claims:          strings.Join(w.implicitClaims, ","),
		Certificate:     marshalledCert,
	}

	if w.ussh != nil {
		req.USSHCertificate = ssh.MarshalAuthorizedKey(w.ussh)
		toSign, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("error marshalling request to sign: %v", err)
		}
		sig, err := w.sshSignMessage(toSign)
		if err != nil {
			return nil, fmt.Errorf("error ssh signing request: %v", err)
		}
		req.Signature = sig.Blob
		req.USSHSignatureType = sig.Format
	} else {
		toSign, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("error marshalling request to sign: %v", err)
		}

		req.Signature, err = wonkacrypter.New().Sign(toSign, key)
		if err != nil {
			return nil, fmt.Errorf("error signing resolve request: %v", err)
		}
	}

	var resp ClaimResponse
	if err := w.httpRequest(ctx, resolveEndpoint, req, &resp); err != nil {
		return nil, fmt.Errorf("error from %s: %v", resolveEndpoint, err)
	}

	tok, err := base64.StdEncoding.DecodeString(resp.Token)
	if err != nil {
		w.log.Error("error decoding token", zap.Error(err))
		return nil, fmt.Errorf("base64 decoding token: %v", err)
	}

	decryptedClaim, err := wonkacrypter.DecryptAny(tok, key, WonkaMasterPublicKeys)
	if err != nil {
		return nil, err
	}

	var c Claim
	if err := json.Unmarshal(decryptedClaim, &c); err != nil {
		return nil, fmt.Errorf("unmarshalling token: %v", err)
	}

	return &c, nil
}

// requestClaim will submit a claim request for the entity, or for the impersonated entity on behalf of the entity,
// and return the wonka claim
func (w *uberWonka) requestClaim(ctx context.Context, impersonatedEntity string, dest string, ttl time.Duration) (*Claim, error) {
	entity := &Entity{
		Requires: wonkaMasterRequires,
	}

	e := w.entityName
	if impersonatedEntity != "" {
		e = impersonatedEntity
	}

	// prime claimsRequested with an identity/everyone claim
	claimsRequested := fmt.Sprintf("%s,%s", EveryEntity, e)

	if strings.ToLower(dest) != wonkaMasterEntity {
		var err error
		entity, err = w.Lookup(ctx, dest)
		if err == nil {
			claimsRequested = entity.Requires
		}
	}

	cr := ClaimRequest{
		Version:            SignEverythingVersion,
		EntityName:         w.entityName,
		ImpersonatedEntity: impersonatedEntity,
		Claim:              claimsRequested,
		Destination:        dest,
		Ctime:              time.Now().Add(-clockSkew).Unix(),
		Etime:              time.Now().Add(ttl).Unix(),
		SigType:            SHA256,
	}

	c, err := w.doRequestClaim(ctx, cr)
	if err != nil {
		w.log.Error("claim request error",
			zap.Error(err),
			zap.Any("destination_entity", dest),
			zap.Any("claim", cr.Claim),
			zap.Any("impersonated_entity", impersonatedEntity),
		)

		return nil, fmt.Errorf("claim request error: %v", err)
	}
	return c, nil

}

// ClaimImpersonateTTL will try to request a claim good for the named entity, on behalf of an impersonated user
func (w *uberWonka) ClaimImpersonateTTL(ctx context.Context, impersonatedEntity string, entityName string, ttl time.Duration) (*Claim, error) {
	return w.requestClaim(ctx, impersonatedEntity, entityName, ttl)
}
