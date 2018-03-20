package main

import (
	"flag"
	"reflect"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
	"github.com/golang/dep/uber"
)

const bootConfigHelp = `Bootstrap the Uber specific default config`

func (cmd *bootConfigCommand) Name() string { return "bootConfig" }
func (cmd *bootConfigCommand) Args() string {
	return ""
}
func (cmd *bootConfigCommand) ShortHelp() string { return bootConfigHelp }
func (cmd *bootConfigCommand) LongHelp() string  { return bootConfigHelp }
func (cmd *bootConfigCommand) Hidden() bool      { return false }

func (cmd *bootConfigCommand) Register(fs *flag.FlagSet) {}

type bootConfigCommand struct{}

func (cmd *bootConfigCommand) Run(ctx *dep.Ctx, args []string) error {
	return BootConfig(ctx)
}

func BootConfig(ctx *dep.Ctx) error {
	curPkgs, basicExcludeDirs, err := base.ReadCustomConfig(ctx.WorkingDir)
	if err != nil {
		return errors.Wrapf(err,"error loading current config")
	}

	impPkgs, err := appendBasicOverrides(curPkgs)
	if err != nil {
		_, ok1 := err.(base.ReferenceOverrideAlreadyExistsForBasic)
		_, ok2 := err.(base.SourceOverrideAlreadyExistsForBasic)
		if ok1 || ok2 {
			uber.UberLogger.Printf("basic override already exists: %s", err.Error())
			return nil
		} else {
			return errors.Wrapf(err,"error appending basic overrides")
		}
	}

	if !reflect.DeepEqual(curPkgs, impPkgs) {
		err = base.WriteCustomConfig(ctx.WorkingDir, impPkgs, base.AppendBasicExcludeDirs(basicExcludeDirs),
			true, ctx.Out)
		if err != nil {
			return errors.Wrapf(err,"error writing custom config at %s", ctx.WorkingDir)
		}
	} else {
		uber.UberLogger.Println("Not writing custom config since nothing to add...")
	}

	return nil
}

func appendBasicOverrides(curPkgs []base.ImportedPackage) ([]base.ImportedPackage, error) {
	pkgSeen := make(map[string]bool)
	for _, curPkg := range curPkgs {
		pkgSeen[curPkg.Name] = true
	}

	return base.AppendBasicOverrides(curPkgs, pkgSeen)
}