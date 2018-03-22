package main

import (
	"flag"

	"github.com/golang/dep"
	"github.com/golang/dep/uber/glide"
	"github.com/pkg/errors"
)

const syncManifestHelp = `Sync the dep manifest with the glide manifest for backwards compatibility`

func (cmd *syncManifestCommand) Name() string { return "syncManifest" }
func (cmd *syncManifestCommand) Args() string {
	return ""
}
func (cmd *syncManifestCommand) ShortHelp() string { return syncManifestHelp }
func (cmd *syncManifestCommand) LongHelp() string  { return syncManifestHelp }
func (cmd *syncManifestCommand) Hidden() bool      { return false }

func (cmd *syncManifestCommand) Register(fs *flag.FlagSet) {}

type syncManifestCommand struct{}

func (cmd *syncManifestCommand) Run(ctx *dep.Ctx, args []string) error {
	err := SyncManifest(ctx.WorkingDir)
	return err
}

func SyncManifest(dir string) error {
	var depAnalyzer dep.Analyzer
	if depAnalyzer.HasDepMetadata(dir) {
		depManifest, _, err := depAnalyzer.DeriveManifestAndLock(dir, "")
		if err != nil {
			return errors.Wrapf(err, "failed to derive manifest")
		}
		err = glide.UpdateGlideArtifacts(depManifest, dir)
		if err != nil {
			return errors.Wrapf(err, "failed to update glide manifest")
		}
	} else {
		return errors.New("directory contains no dep metadata")
	}

	return nil
}
