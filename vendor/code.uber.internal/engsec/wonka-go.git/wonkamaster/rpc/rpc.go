package rpc

import (
	"fmt"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/.gen/go/pullo"
	"code.uber.internal/engsec/wonka-go.git/internal/logging"

	"github.com/uber/tchannel-go"
	tthrift "github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
)

const defaultMuttleyHostPort = "127.0.0.1:5437"

// PulloClient is a pullo connection for getting group information on a user.
type PulloClient interface {
	GetGroupsForUser(user string) []string
	IsMemberOf(user, group string) bool
}

// NOTE(abg): This could totally just use YARPC instead.

// wmPulloClient implements the PulloClient interface.
type wmPulloClient struct {
	log *zap.Logger
	ch  *tchannel.Channel
}

type pulloClientConfig struct {
	Logger *zap.Logger
	Level  zap.AtomicLevel

	// If unset, defaultMuttleyHostPort will be used.
	MuttleyHostPort string
}

func (cfg *pulloClientConfig) BuildTChannel() (*tchannel.Channel, error) {
	var opts tchannel.ChannelOptions
	if cfg.Logger != nil {
		opts.Logger = logging.NewTChannelLogger(cfg.Logger, cfg.Level)
	}

	ch, err := tchannel.NewChannel("wonkamaster", &opts)
	if err != nil {
		return nil, err
	}

	ch.Peers().Add(cfg.MuttleyHostPort)
	return ch, nil
}

// PulloClientOption customizes the behavior of a Pullo client.
type PulloClientOption func(*pulloClientConfig)

// Logger specifies how messages will be logged.
func Logger(log *zap.Logger, level zap.AtomicLevel) PulloClientOption {
	return func(cfg *pulloClientConfig) {
		cfg.Logger = log
		cfg.Level = level
	}
}

// MuttleyHostPort specifies the address at which the router for Pullo
// requests is available.
func MuttleyHostPort(hostPort string) PulloClientOption {
	return func(cfg *pulloClientConfig) {
		cfg.MuttleyHostPort = hostPort
	}
}

// NewPulloClient returns a new pullo client object
func NewPulloClient(opts ...PulloClientOption) (PulloClient, error) {
	cfg := pulloClientConfig{
		Logger:          zap.L(),
		Level:           zap.NewAtomicLevel(),
		MuttleyHostPort: defaultMuttleyHostPort,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	ch, err := cfg.BuildTChannel()
	if err != nil {
		return nil, fmt.Errorf("failed to set up TChannel: %v", err)
	}

	return &wmPulloClient{
		log: cfg.Logger,
		ch:  ch,
	}, nil
}

func (u *wmPulloClient) IsMemberOf(user, group string) bool {
	tClient := pullo.NewTChanPulloClient(tthrift.NewClient(u.ch, "pullo", nil))
	ctx, _ := tchannel.NewContextBuilder(60*time.Second).
		AddHeader("X-Uber-Source", "wonkamaster").
		Build()

	ok, err := tClient.IsMemberOf(ctx, user, group)
	if err != nil {
		u.log.Error("error making pullo request", zap.Error(err))
		return false
	}

	return ok
}

// we should be using titus for this.
func (u *wmPulloClient) GetGroupsForUser(user string) []string {
	adGroups := []string{wonka.EveryEntity}

	tClient := pullo.NewTChanPulloClient(tthrift.NewClient(u.ch, "pullo", nil))
	ctx, _ := tchannel.NewContextBuilder(60*time.Second).
		AddHeader("X-Uber-Source", "wonkamaster").
		Build()

	groups, err := tClient.GetUserGroups(ctx, user)
	if err != nil {
		u.log.Error("error querying pullo", zap.Any("user", user), zap.Error(err))
		return adGroups
	}

	for _, g := range groups {
		if g == "" {
			continue
		}
		adGroups = append(adGroups, fmt.Sprintf("AD:%s", g))
	}

	return adGroups
}
