package galileo

import "errors"

// SetConfigGalileo extends Galileo interface with methods that allow for
// dynamic configuration of the Galileo object.
type SetConfigGalileo interface {
	Galileo
	SetConfig(options ...ConfigOption) error
}

var _ SetConfigGalileo = (*uberGalileo)(nil)

type config struct {
	Percent *float64
}

// ConfigOption may be passed to SetConfig to dynamically update
// some aspect of the current enforcement.
type ConfigOption interface {
	apply(*config) error
}

// EnforcePercentage atomically sets the current enforcement level.
func EnforcePercentage(percent float64) ConfigOption {
	return enforcePercentage(percent)
}

type enforcePercentage float64

func (e enforcePercentage) apply(cfg *config) error {
	percent := float64(e)
	if err := validateEnforcePercentage(percent); err != nil {
		return err
	}
	cfg.Percent = &percent
	return nil
}

// SetConfig applies the config options onto the Galileo object.
func SetConfig(g Galileo, opts ...ConfigOption) error {
	eg, ok := g.(SetConfigGalileo)
	if !ok {
		return errors.New("galileo instance does not implement galileo.SetConfigGalileo interface")
	}
	return eg.SetConfig(opts...)
}

// SetConfig applies the enforce options.
func (u *uberGalileo) SetConfig(opts ...ConfigOption) error {
	var cfg config
	for _, opt := range opts {
		if err := opt.apply(&cfg); err != nil {
			return err
		}
	}
	if cfg.Percent != nil {
		u.enforcePercentage.Store(*cfg.Percent)
	}
	return nil
}

func validateEnforcePercentage(percentage float64) error {
	if percentage < 0 || percentage > 1 {
		return errors.New("enforce percentage should be from [0.0, 1.0]")
	}
	return nil
}
