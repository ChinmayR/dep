package main

import (
	"flag"

	"github.com/golang/dep"
	"github.com/golang/dep/uber"
	"github.com/pkg/errors"
)

const cacheClearShortHelp = `clear the dep cache at DEPCACHEDIR env var or $HOME/.dep-cache/pkg/dep`
const cacheClearLongHelp = `clear the dep cache at DEPCACHEDIR env var or $HOME/.dep-cache/pkg/dep`

func (cmd *cacheClearCommand) Name() string      { return "cc" }
func (cmd *cacheClearCommand) Args() string      { return "" }
func (cmd *cacheClearCommand) ShortHelp() string { return cacheClearShortHelp }
func (cmd *cacheClearCommand) LongHelp() string  { return cacheClearLongHelp }
func (cmd *cacheClearCommand) Hidden() bool      { return false }

func (cmd *cacheClearCommand) Register(fs *flag.FlagSet) {}

type cacheClearCommand struct{}

func (cmd *cacheClearCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 0 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	sm.UseDefaultSignalHandling(uber.GetRepoTagFriendlyNameFromCWD(ctx.WorkingDir), cmd.Name())
	defer sm.Release()

	err = sm.ClearCacheDir()
	if err != nil {
		return errors.Wrap(err, "error removing cache dir")
	}

	if err := uber.WriteCacheClearedVersion(uber.DEP_VERSION, sm.Cachedir()); err != nil {
		return errors.Wrap(err, "error writing clear cache file")
	}

	uber.ReportClearCacheMetric(cmd.Name())
	uber.UberLogger.Printf("Cache cleared at %v\n", sm.Cachedir())

	return nil
}
