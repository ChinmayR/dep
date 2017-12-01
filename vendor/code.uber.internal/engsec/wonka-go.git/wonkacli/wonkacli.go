package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacli/cmd"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"github.com/uber-go/tally"
	jaeger "github.com/uber/jaeger-client-go"
	jconfig "github.com/uber/jaeger-client-go/config"
	jzap "github.com/uber/jaeger-client-go/log/zap"
	"github.com/urfave/cli"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/ssh/agent"
)

// If you have a valid ussh cert, you can use wonkacli to enroll a new
// service in wonka like so:
//
//  $ ./wonkacli --private-key=private.pem \
//     enroll fooservice --allowed-groups=AD:engineering
//
// this will create 'fooservice' and set its allowedgroups to be
// AD:engineering. If fooservice already exists, this will fail if the
// private.pem and publice.pem are different from what was previously
// enrolled.
//
// if you need to generate public.pem and private.pem, you can do it like
// this:
//
//  $ openssl genrsa -out private.pem 4096
//  $ openssl rsa -in private.pem -outform PEM -pubout -out public.pem
//
// Now you can upload private.pem to langley and share it with your
// service as a secret named, wonka_private

func main() {
	app := cli.NewApp()
	app.Name = "wonkacli"
	app.Usage = "Service-to-service authN at Uber"
	app.Description = "Provides a command line interface to Wonka."
	app.Version = wonka.Version
	cli.VersionFlag = cli.BoolFlag{Name: "version, V"}
	cli.VersionPrinter = func(c *cli.Context) {
		cmd.Version(c)
	}

	app.Commands = []cli.Command{
		{
			Name:   "version",
			Usage:  "Print the version and exit",
			Action: cmd.Version,
		},

		{
			Name:  "enroll",
			Usage: "Enroll an entity",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "entity, name",
					Usage: "Entity name to enroll",
				},
				cli.StringSliceFlag{
					Name:  "allowed-groups, allowedgroups, g",
					Usage: "Groups permitted to talk to your entity",
				},
				cli.BoolFlag{
					Name:  "generate-keys",
					Usage: "generate wonka_private and wonka_public",
				},
				cli.StringFlag{
					Name:  "private-key,privkey",
					Usage: "private key for the entity to be enrolled",
				},
			},
			Action: cmd.Enroll,
		},

		{
			Name:  "verify",
			Usage: "verify a signature",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "signer",
					Usage: "entity who signed this data",
				},
				cli.StringFlag{
					Name:  "signature, sig",
					Usage: "base64 encoded signature",
				},
				cli.StringFlag{
					Name:  "data, d",
					Usage: "string to sign",
				},
				cli.StringFlag{
					Name:  "file, f",
					Usage: "file to sign",
				},
				cli.BoolFlag{
					Name:  "cert",
					Usage: "this data was signed by a wonkacert",
				},

				// this should be parsed out of the launch request
				cli.StringFlag{
					Name:  "taskid, t",
					Usage: "on verify, the task id for the new cert",
				},
				cli.StringFlag{
					Name:  "key-path, k",
					Usage: "output path for the key",
				},
				cli.StringFlag{
					Name:  "cert-path, c",
					Usage: "output path for the cert",
				},
			},
			Action: cmd.VerifySignature,
		},

		{
			Name:  "sign",
			Usage: "sign a string with the entity private key",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "cert",
					Usage: "request a new cert to perform this signing operation",
				},
				cli.StringFlag{
					Name:  "wonkad-path, w",
					Usage: "if generating a certificate, where to find wonkad",
					Value: "/var/run/wonkad.sock",
				},
				cli.StringFlag{
					Name:  "data, d",
					Usage: "string to sign",
				},
				cli.StringFlag{
					Name:  "file, f",
					Usage: "file to sign",
				},
				cli.DurationFlag{
					Name:  "timeout",
					Usage: "return error if we can't sign data before this timeout",
					Value: 10 * time.Second,
				},
			},
			Action: cmd.SignData,
		},

		{
			Name:  "disable",
			Usage: "Sign a disable message",
			Flags: []cli.Flag{
				cli.DurationFlag{
					Name:  "expiration, e",
					Value: 15 * time.Minute,
					Usage: "how long to disable (note that the message can replaied for this long)",
				},
			},
			Action: cmd.SignDisableMessage,
		},

		{
			Name:  "delete",
			Usage: "Delete an entity",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "entity, name",
					Usage: "Entity name to delete",
				},
			},
			Action: cmd.Delete,
		},

		{
			Name:   "certificate",
			Usage:  "create a wonka certificate ",
			Action: cmd.RequestCertificate,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "taskid, t",
					Usage: "task id for this certificate",
				},
				cli.StringFlag{
					Name:  "x509-path, x",
					Usage: "optional duplicate of private key in x509 format",
				},
				cli.StringFlag{
					Name:  "key-path, k",
					Usage: "output path for the key",
					Value: "wonka.key",
				},
				cli.StringFlag{
					Name:  "cert-path, c",
					Usage: "output path for the cert",
					Value: "wonka.crt",
				},
				cli.StringFlag{
					Name:  "wonkad-path, w",
					Usage: "path to wonkad sock",
					Value: "/var/run/wonkad.sock",
				},
			},
		},

		{
			Name:   "task",
			Usage:  "Sign and verify mesos/docker launch requests",
			Action: cmd.Task,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "sign",
					Usage: "The launch request to sign. Cannot be used with -verify.",
				},
				cli.StringFlag{
					Name:  "verify",
					Usage: "The launch request siganture to verify. Cannont be used with -sign.",
				},
				cli.StringFlag{
					Name:  "certificate",
					Usage: "When signing, this is the certificate with which to sign. When verifying, this is where to place the resulting certificate.",
				},
				cli.StringFlag{
					Name:  "key",
					Usage: "When signing, this is the private key with which to sign. When verifying, this is where to place the resulting private key.",
				},
			},
		},

		{
			Name:    "validate",
			Aliases: []string{"check"},
			Usage:   "Validate that the provided token is cryptographically correct",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "token, t",
					Usage: "The token to validate",
				},
				cli.StringSliceFlag{
					Name:  "claim-list, L",
					Usage: "Assert the provided token is valid for one of the values in the claim-list",
				},
			},
			Action: cmd.Validate,
		},

		{
			Name:  "lookup",
			Usage: "Look up an entity in Wonkamaster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "entity, name",
					Usage: "Entity name to lookup",
				},
			},
			Action: cmd.Lookup,
		},

		{
			Name:  "request",
			Usage: "Request a token asserting the provided claim",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "claim, name",
					Usage: "Claim name to request a token for",
				},
				cli.StringFlag{
					Name:  "destination, dest, to",
					Usage: "Destination for this claim (default: your personnel)",
				},
				cli.StringFlag{
					Name:  "source, src, from",
					Usage: "Source entity to impersonate (only authorized services can do this)",
				},
				cli.StringFlag{
					Name:  "output, o",
					Usage: "File to write the resulting claim",
				},
				cli.DurationFlag{
					Name:  "expiration, e",
					Value: 15 * time.Minute,
					Usage: "Lifetime (duration) for this claim",
				},
			},
			Action: cmd.Request,
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "self",
			Value:  os.Getenv("UBER_OWNER"),
			Usage:  "your entity name",
			EnvVar: "WONKACLI_SELF",
		},
		cli.BoolFlag{
			Name:   "verbose, v",
			Usage:  "verbose output",
			EnvVar: "WONKACLI_VERBOSE",
		},
		cli.StringFlag{
			Name:  "private-key, privkey",
			Usage: "Path to private key to use, formatted as PEM. If unset, use session keys",
		},
		cli.StringFlag{
			Name:  "wonkamasterurl,url",
			Usage: "Wonkamaster url",
		},
		cli.BoolFlag{
			Name:  "json, j",
			Usage: "Input and output will be in JSON, not Base64",
		},
	}

	var log *zap.Logger
	var tracer opentracing.Tracer
	var span opentracing.Span
	var closer io.Closer

	app.Before = func(c *cli.Context) error {
		logCfg := zap.NewDevelopmentConfig()
		if !c.GlobalBool("verbose") {
			logCfg.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
			logCfg.DisableStacktrace = true
		}

		var err error
		log, err = logCfg.Build()
		if err != nil {
			return fmt.Errorf("failed to set up logger: %v", err)
		}
		zap.ReplaceGlobals(log)

		tracer, closer, err = newTracer(app.Name, log)
		if err != nil {
			return fmt.Errorf("error initializing jaeger: %v", err)
		}
		span = tracer.StartSpan(app.Name)
		ctx := opentracing.ContextWithSpan(context.Background(), span)

		formatter := &prefixed.TextFormatter{
			TimestampFormat: "15:04:05.00",
			FullTimestamp:   true,
		}
		logrus.SetFormatter(formatter)

		var a agent.Agent
		agentSock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		if err == nil {
			a = agent.NewClient(agentSock)
		}

		cfg := wonka.Config{
			EntityName:     c.GlobalString("self"),
			PrivateKeyPath: c.GlobalString("private-key"),
			Logger:         log,
			Metrics:        tally.NoopScope,
			Agent:          a,
			Tracer:         tracer,
		}

		c.App.Metadata["config"] = cfg
		c.App.Metadata["ctx"] = ctx

		return nil
	}

	app.After = func(c *cli.Context) error {
		if span != nil {
			span.Finish()
		}
		if closer != nil {
			closer.Close()
		}
		if log != nil {
			return log.Sync()
		}
		return nil
	}

	app.Run(os.Args)
}

func newTracer(name string, log *zap.Logger) (opentracing.Tracer, io.Closer, error) {
	jlog := jzap.NewLogger(log)
	jaegerConfig := jconfig.Configuration{
		Sampler: &jconfig.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jconfig.ReporterConfig{
			LogSpans: true,
		},
	}
	return jaegerConfig.New(
		name,
		jconfig.Logger(jlog),
	)
}
