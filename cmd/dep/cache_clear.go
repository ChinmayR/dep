package main

import (
	"flag"

	"github.com/golang/dep"
	"github.com/pkg/errors"
	"github.com/golang/dep/uber"
)

const cacheClearShortHelp = `clear the dep cache at $HOME/.dep_cache/pkg/dep`
const cacheClearLongHelp = `clear the dep cache at $HOME/.dep_cache/pkg/dep`

func (cmd *cacheClearCommand) Name() string      { return "cc" }
func (cmd *cacheClearCommand) Args() string      { return "" }
func (cmd *cacheClearCommand) ShortHelp() string { return cacheClearShortHelp }
func (cmd *cacheClearCommand) LongHelp() string  { return cacheClearLongHelp }
func (cmd *cacheClearCommand) Hidden() bool      { return false }

func (cmd *cacheClearCommand) Register(fs *flag.FlagSet) {}

type cacheClearCommand struct {}

func (cmd *cacheClearCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 0 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	err = sm.ClearCacheDir()
	if err != nil {
		return errors.Wrap(err, "error removing cache dir")
	}

	uber.ReportClearCacheMetric(cmd.Name())
	uber.UberLogger.Println("Cache cleared at $HOME/.dep_cache/pkg/dep")

	return nil
}
