package health

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// Version is the current package version.
	Version = "1.0.0"

	// DefaultCoolDown is the default amount of time that a Coordinator will wait
	// in the Stopping phase before entering Stopped. It's chosen with Uber's
	// Health Controller system in mind, since it can take up to ten seconds for
	// health state changes to propagate.
	DefaultCoolDown = 10 * time.Second
)

// State represents the current health state of the process.
type State int

const (
	// RefusingTraffic is the default state, indicating that the process is
	// healthy but doesn't want to receive traffic.
	RefusingTraffic State = iota
	// AcceptingTraffic indicates that the process is healthy and willing to
	// receive traffic.
	AcceptingTraffic
	// Stopping indicates that the process is about to terminate. Typically,
	// RPC servers enter this state and pause for a few seconds before draining
	// in-flight requests and actually shutting down; the cool-down period
	// allows information to propagate through Uber's health controller system.
	//
	// Stopping processes always progress to being Stopped, and can't reverse
	// course to RefusingTraffic or AcceptingTraffic.
	Stopping
	// Stopped indicates that the process has waited through its cool-down
	// period and is ready to drain in-flight requests and shut down.
	//
	// Stopped is a terminal state.
	Stopped
)

// String implements fmt.Stringer.
func (s State) String() string {
	switch s {
	case RefusingTraffic:
		return "refusing"
	case AcceptingTraffic:
		return "accepting"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	default:
		return fmt.Sprintf("State(%d)", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (s *State) UnmarshalText(text []byte) error {
	if s == nil {
		return errors.New("can't unmarshal a nil *State")
	}
	str := string(text)
	switch strings.ToLower(str) {
	case "refusing":
		*s = RefusingTraffic
	case "accepting":
		*s = AcceptingTraffic
	case "stopping":
		*s = Stopping
	case "stopped":
		*s = Stopped
	default:
		return fmt.Errorf("unknown state %q", str)
	}
	return nil
}

// A Coordinator allows multiple network servers to coordinate their health
// state. It uses that consistent view of state to serve both Nagios- and
// Health Controller-style health checks.
//
// If a single process has multiple network servers (e.g., a legacy REST HTTP
// server and a YARPC dispatcher), they should share a reference to the same
// Coordinator.
type Coordinator struct {
	stateMu    sync.RWMutex
	stateTimer *time.Timer
	state      State

	wait    time.Duration
	name    string
	stopped chan struct{}
}

// An Option configures a Coordinator.
type Option interface {
	apply(*Coordinator)
}

type optionFunc func(*Coordinator)

func (f optionFunc) apply(c *Coordinator) { f(c) }

// CoolDown changes the Coordinator's cool-down period. It's useful in
// development, where waiting ten seconds for a server to shut down is
// irritating.
func CoolDown(d time.Duration) Option {
	return optionFunc(func(c *Coordinator) {
		c.wait = d
	})
}

// NewCoordinator returns a new Coordinator in the RefusingTraffic state.
func NewCoordinator(name string, opts ...Option) *Coordinator {
	c := &Coordinator{
		name:    name,
		wait:    DefaultCoolDown,
		stopped: make(chan struct{}),
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	return c
}

// State returns the current health state.
func (c *Coordinator) State() State {
	// Health checks only run a few times per second; no need to optimize away
	// defers.
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// AcceptTraffic transitions to the AcceptingTraffic state. It returns an
// error if called when the process is already Stopping or Stopped.
func (c *Coordinator) AcceptTraffic() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.isShuttingDown() {
		return fmt.Errorf("can't start accepting traffic in state %q", c.state)
	}
	c.state = AcceptingTraffic
	return nil
}

// RefuseTraffic transitions to the RefusingTraffic state. It returns an error
// if called when the process is already Stopping or Stopped.
func (c *Coordinator) RefuseTraffic() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.isShuttingDown() {
		return fmt.Errorf("can't start refusing traffic in state %q", c.state)
	}
	c.state = RefusingTraffic
	return nil
}

// Stop transitions to Stopping. Ten seconds later, the process automatically
// transitions to Stopped.
//
// If the process is already Stopping or Stopped, calling Stop is a no-op.
func (c *Coordinator) Stop() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.isShuttingDown() {
		return
	}
	c.state = Stopping
	c.stateTimer = time.AfterFunc(c.wait, func() {
		c.stateMu.Lock()
		c.state = Stopped
		close(c.stopped)
		c.stateMu.Unlock()
	})
}

// Stopped returns a channel that unblocks when the process enters the
// Stopped state.
func (c *Coordinator) Stopped() <-chan struct{} {
	return c.stopped
}

// Cancel any outstanding timers. Useful to avoid leaks in unit tests.
func (c *Coordinator) cleanup() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.stateTimer != nil {
		c.stateTimer.Stop()
	}
}

func (c *Coordinator) isShuttingDown() bool {
	return c.state == Stopping || c.state == Stopped
}
