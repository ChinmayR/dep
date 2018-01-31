package cmd

import (
	"crypto"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/sirupsen/logrus"
)

// Reusable utility functions not specific to this CLI app

func marshalClaimJSON(claim *wonka.Claim) (string, error) {
	claimBytes, err := json.Marshal(claim)
	if err != nil {
		return "", err
	}

	return string(claimBytes), nil
}

func logWithEntity(entity wonka.Entity, msg string) {
	pubKey := ""
	keyBytes, err := base64.StdEncoding.DecodeString(entity.PublicKey)
	if err != nil {
		pubKey = "invalid key"
	} else {
		hasher := crypto.SHA256.New()
		hasher.Write(keyBytes)
		pubKey = base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	}

	logrus.WithFields(logrus.Fields{
		"name":     strconv.QuoteToASCII(entity.EntityName),
		"requires": strconv.QuoteToASCII(entity.Requires),
		"ecc":      entity.ECCPublicKey,
		"pubkey":   pubKey,
		"ctime":    time.Unix(int64(entity.Ctime), 0),
		"version":  entity.Version,
	}).Info(msg)
}

func logWithClaim(claim wonka.Claim, msg string) {
	logrus.WithFields(logrus.Fields{
		"claim":       strings.Join(claim.Claims, ","),
		"destination": claim.Destination,
		"validAfter":  time.Unix(claim.ValidAfter, 0),
		"validBefore": time.Unix(claim.ValidBefore, 0),
		"validFor":    time.Unix(claim.ValidBefore, 0).Sub(time.Unix(claim.ValidAfter, 0)),
		"signature":   base64.StdEncoding.EncodeToString(claim.Signature),
	}).Info(msg)
}

func wonkadRequest(path string, req wonka.WonkadRequest) (wonka.WonkadReply, error) {
	var repl wonka.WonkadReply
	conn, err := net.Dial("unix", path)
	if err != nil {
		return repl, fmt.Errorf("error contacting wonkad: %v", err)
	}
	defer conn.Close()

	toWrite, err := json.Marshal(req)
	if err != nil {
		return repl, fmt.Errorf("error marshalling request: %v", err)
	}

	if _, err := conn.Write(toWrite); err != nil {
		return repl, fmt.Errorf("error writing request: %v", err)
	}

	b, err := ioutil.ReadAll(conn)
	if err != nil {
		return repl, fmt.Errorf("error reading reply: %v", err)
	}

	if err := json.Unmarshal(b, &repl); err != nil {
		return repl, fmt.Errorf("error unmarshalling reply: %v", err)
	}

	return repl, nil
}
