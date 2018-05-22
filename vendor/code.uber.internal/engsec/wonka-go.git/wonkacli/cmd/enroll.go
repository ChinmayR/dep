package cmd

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.uber.org/zap"
)

var (
	enrollMsg = `

enrollment succeeded! you're almost done.
now you need to add your service keys to langley"
1. add wonka_private (your private key), name it 'wonka_private', and share it with your service
2. add wonka_public (your public key), name it 'wonka_public', and share it with your service
3. if enrolling more services, delete wonka_private and wonka_public before re-running
`
)

func performEnroll(c CLIContext) error {
	log := zap.L()

	entity := c.StringOrFirstArg("entity")
	if entity == "" {
		log.Error("no entity provided to enroll")
		return cli.NewExitError("", 1)
	}

	allowedGroups := c.StringSlice("allowed-groups")
	if len(allowedGroups) == 0 {
		allowedGroups = []string{wonka.EveryEntity}
	}

	log.Info("enrolling an entity",
		zap.Any("entity", entity),
		zap.Any("allowGroups", allowedGroups),
	)

	exitMsg := false
	clientType := DefaultClient
	if c.Bool("generate-keys") {
		exitMsg = true
		logrus.Warn("generating keys can take a few seconds")
		clientType = EnrollmentClientGenerateKeys
	}

	w, err := c.NewWonkaClient(clientType)
	if err != nil {
		logrus.WithField("error", err).Error("error enrolling")
		return cli.NewExitError("", 1)
	}

	// helper
	kh := c.NewKeyHelper()
	privKeyPath := c.String("private-key")
	if privKeyPath == "" {
		privKeyPath = "wonka_private"
	}
	rsaPriv, rsaPub, eccPub, err := kh.RSAAndECC(privKeyPath)
	if err != nil {
		logrus.WithField("error", err).Error("error getting keys")
		return cli.NewExitError("", 1)
	}

	requires := strings.Join(c.StringSlice("allowed-groups"), ",")
	if len(requires) == 0 {
		requires = wonka.EveryEntity
	}

	newEntity := &wonka.Entity{
		EntityName:   c.StringOrFirstArg("entity"),
		PublicKey:    rsaPub,
		ECCPublicKey: eccPub,
		Version:      wonka.SignEverythingVersion,
		SigType:      "SHA256",
		Requires:     requires,
		Ctime:        int(time.Now().Unix()),
	}

	toSign := fmt.Sprintf("%s<%d>%s", newEntity.EntityName, newEntity.Ctime, newEntity.PublicKey)
	h := crypto.SHA256.New()
	h.Write([]byte(toSign))
	sig, err := rsaPriv.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"self":   c.GlobalString("self"),
			"enroll": entity,
			"error":  err,
		}).Error("error signing new entity")
		return cli.NewExitError("", 1)
	}
	newEntity.EntitySignature = base64.StdEncoding.EncodeToString(sig)

	result, err := w.EnrollEntity(c.Context(), newEntity)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"self":   c.GlobalString("self"),
			"enroll": entity,
			"error":  err,
		}).Error("error enrolling")
		return cli.NewExitError("", 1)
	}

	logWithEntity(*result, "entity enrolled")

	if exitMsg {
		logrus.Info(enrollMsg)
	}

	return nil
}

// Enroll an entity
func Enroll(c *cli.Context) error {
	return performEnroll(cliWrapper{inner: c})
}
