// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/importers/base"
	"github.com/golang/dep/uber"
	"github.com/golang/dep/uber/glide"
	"github.com/pkg/errors"
)

const initShortHelp = `Set up a new Go project, or migrate an existing one`
const initLongHelp = `
Initialize the project at filepath root by parsing its dependencies, writing
manifest and lock files, and vendoring the dependencies. If root isn't
specified, use the current directory.

When configuration for another dependency management tool is detected, it is
imported into the initial manifest and lock. Use the -skip-tools flag to
disable this behavior. The following external tools are supported:
glide, godep, vndr, govend, gb, gvt, govendor, glock.

Any dependencies that are not constrained by external configuration use the
GOPATH analysis below.

By default, the dependencies are resolved over the network. A version will be
selected from the versions available from the upstream source per the following
algorithm:

 - Tags conforming to semver (sorted by semver rules)
 - Default branch(es) (sorted lexicographically)
 - Non-semver tags (sorted lexicographically)

An alternate mode can be activated by passing -gopath. In this mode, the version
of each dependency will reflect the current state of the GOPATH. If a dependency
doesn't exist in the GOPATH, a version will be selected based on the above
network version selection algorithm.

A Gopkg.toml file will be written with inferred version constraints for all
direct dependencies. Gopkg.lock will be written with precise versions, and
vendor/ will be populated with the precise versions written to Gopkg.lock.
`

func (cmd *initCommand) Name() string      { return "init" }
func (cmd *initCommand) Args() string      { return "[root]" }
func (cmd *initCommand) ShortHelp() string { return initShortHelp }
func (cmd *initCommand) LongHelp() string  { return initLongHelp }
func (cmd *initCommand) Hidden() bool      { return false }

func (cmd *initCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.noExamples, "no-examples", false, "don't include example in Gopkg.toml")
	fs.BoolVar(&cmd.skipTools, "skip-tools", false, "skip importing configuration from other dependency managers")
	fs.BoolVar(&cmd.gopath, "gopath", false, "search in GOPATH for dependencies")
	fs.BoolVar(&cmd.withMirror, "withMirror", false, "enable github mirroring internally in gitolite")
}

type initCommand struct {
	noExamples bool
	skipTools  bool
	gopath     bool
	withMirror bool
}

func (cmd *initCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 1 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	// this flag controls bootstrapping the custom config when running outside of integration tests
	if os.Getenv(uber.RunningIntegrationTests) == "" {
		err := BootConfig(ctx)
		if err != nil {
			uber.UberLogger.Printf("Failed to boot custom config, run \"dep bootConfig\" manually: %s", err)
		}
	}

	// this flag controls if external github repos need to be mirrored internally at gitolite
	if !cmd.withMirror {
		uber.UberLogger.Println("Internal mirroring is turned off for performance optimization. Run with --withMirror flag to mirror into gitolite")
		os.Setenv(uber.UberDisableGitoliteAutocreation, "yes")
	}

	var root string
	if len(args) <= 0 {
		root = ctx.WorkingDir
	} else {
		root = args[0]
		if !filepath.IsAbs(args[0]) {
			root = filepath.Join(ctx.WorkingDir, args[0])
		}
		if err := os.MkdirAll(root, os.FileMode(0777)); err != nil {
			return errors.Wrapf(err, "init failed: unable to create a directory at %s", root)
		}
	}

	flags := make(map[string]string)
	flags["gopath"] = strconv.FormatBool(cmd.gopath)
	flags["noexamples"] = strconv.FormatBool(cmd.noExamples)
	flags["skiptools"] = strconv.FormatBool(cmd.skipTools)
	defer uber.ReportRepoMetrics(cmd.Name(), ctx.WorkingDir, flags)()

	p, err := cmd.establishProjectAt(root, ctx)
	if err != nil {
		return err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "init failed: unable to create a source manager")
	}
	sm.UseDefaultSignalHandling(uber.GetRepoTagFriendlyNameFromCWD(ctx.WorkingDir), cmd.Name())
	defer sm.Release()

restart:
	if ctx.Verbose {
		ctx.Out.Println("Getting direct dependencies...")
	}

	ptree, directDeps, err := p.GetDirectDependencyNames(sm)
	if err != nil {
		return errors.Wrap(err, "init failed: unable to determine direct dependencies")
	}
	if ctx.Verbose {
		ctx.Out.Printf("Checked %d directories for packages.\nFound %d direct dependencies.\n", len(ptree.Packages), len(directDeps))
	}

	// Initialize with imported data, then fill in the gaps using the GOPATH
	rootAnalyzer := newRootAnalyzer(cmd.skipTools, ctx, directDeps, sm)
	p.Manifest, p.Lock, err = rootAnalyzer.InitializeRootManifestAndLock(root, p.ImportRoot)
	if err != nil {
		return errors.Wrap(err, "init failed: unable to prepare an initial manifest and lock for the solver")
	}

	// Set default prune options to false, leaving the repo owner to prune as needed
	p.Manifest.PruneOptions.DefaultOptions = 0

	if cmd.gopath {
		gs := newGopathScanner(ctx, directDeps, sm)
		err = gs.InitializeRootManifestAndLock(p.Manifest, p.Lock)
		if err != nil {
			return errors.Wrap(err, "init failed: unable to scan the GOPATH for dependencies")
		}
	}

	rootAnalyzer.skipTools = importDuringSolve()
	copyLock := *p.Lock // Copy lock before solving. Use this to separate new lock projects from solved lock

	params := gps.SolveParameters{
		RootDir:         root,
		RootPackageTree: ptree,
		Manifest:        p.Manifest,
		Lock:            p.Lock,
		ProjectAnalyzer: rootAnalyzer,
	}

	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	}

	if err := ctx.ValidateParams(sm, params); err != nil {
		return errors.Wrapf(err, "init failed: validation of solve parameters failed")
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "init failed: unable to prepare the solver")
	}

	soln, err := s.Solve(context.TODO())
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		errInternal := handleSolveConflicts(ctx, err)
		if errInternal != nil {
			return err
		}
		goto restart
	}
	p.Lock = dep.LockFromSolution(soln)

	rootAnalyzer.FinalizeRootManifestAndLock(p.Manifest, p.Lock, copyLock)

	// Run gps.Prepare with appropriate constraint solutions from solve run
	// to generate the final lock memo.
	s, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "init failed: unable to recalculate the lock digest")
	}

	p.Lock.SolveMeta.InputsDigest = s.HashInputs()

	sw, err := dep.NewSafeWriter(p.Manifest, nil, p.Lock, dep.VendorAlways, p.Manifest.PruneOptions)
	if err != nil {
		return errors.Wrap(err, "init failed: unable to create a SafeWriter")
	}

	var logger *log.Logger
	if ctx.Verbose {
		logger = ctx.Err
	}
	if err := sw.Write(root, sm, !cmd.noExamples, logger); err != nil {
		return errors.Wrap(err, "init failed: unable to write the manifest, lock and vendor directory to disk")
	}

	if err := glide.UpdateGlideArtifacts(sw.Manifest, root); err != nil {
		return errors.Wrap(err, "writing dep maintained glide file")
	}

	// Divide the total latency by the number of projects
	if p.Lock != nil {
		uber.LatencyNormFactor(len(p.Lock.Projects()))
	}
	uber.ReportSuccess()
	// only remove the config at the end if the init run is successful
	RemoveConfig(ctx)

	return nil
}

// establishProjectAt attempts to set up the provided path as the root for the
// project to be created.
//
// It checks for being within a GOPATH, that there is no pre-existing manifest
// and lock, and that we can successfully infer the root import path from
// GOPATH.
//
// If successful, it returns a dep.Project, ready for further use.
func (cmd *initCommand) establishProjectAt(root string, ctx *dep.Ctx) (*dep.Project, error) {
	var err error
	p := new(dep.Project)
	if err = p.SetRoot(root); err != nil {
		return nil, errors.Wrapf(err, "init failed: unable to set the root project to %s", root)
	}

	ctx.GOPATH, err = ctx.DetectProjectGOPATH(p)
	if err != nil {
		return nil, errors.Wrapf(err, "init failed: unable to detect the containing GOPATH")
	}

	mf := filepath.Join(root, dep.ManifestName)
	lf := filepath.Join(root, dep.LockName)

	mok, err := fs.IsRegular(mf)
	if err != nil {
		return nil, errors.Wrapf(err, "init failed: unable to check for an existing manifest at %s", mf)
	}
	if mok {
		return nil, errors.Errorf("init aborted: manifest already exists at %s", mf)
	}

	lok, err := fs.IsRegular(lf)
	if err != nil {
		return nil, errors.Wrapf(err, "init failed: unable to check for an existing lock at %s", lf)
	}
	if lok {
		return nil, errors.Errorf("invalid aborted: lock already exists at %s", lf)
	}

	ip, err := ctx.ImportForAbs(root)
	if err != nil {
		return nil, errors.Wrapf(err, "init failed: unable to determine the import path for the root project %s", root)
	}
	p.ImportRoot = gps.ProjectRoot(ip)

	return p, nil
}

func handleSolveConflicts(ctx *dep.Ctx, err error) error {
	ovrPkgs, errInternal := gps.HandleErrors(ctx.Out, err)
	if errInternal != nil {
		ctx.Err.Println(errInternal)
		return errInternal
	}
	if len(ovrPkgs) == 0 {
		return errors.New("No resolution options to provide")
	}
	ctx.Out.Print("Select an option: ")
	var i int
	fmt.Scan(&i)
	var ovrPkgSelected gps.OverridePackage
	if i == gps.EXIT_NUM {
		ctx.Out.Println("User selected exit")
		return errors.New("User selected exit")
	} else if i == gps.CUSTOM_NUM { //provide an option to set a custom override not in the recommendation list
		ctx.Out.Print("Package Name: ")
		var overName string
		fmt.Scanln(&overName)
		overName = strings.Trim(overName, " ")
		ctx.Out.Print("Override version: ")
		var overVersion string
		fmt.Scanln(&overVersion)
		overVersion = strings.Trim(overVersion, " ")
		ctx.Out.Print("Override source: ")
		var overSource string
		fmt.Scanln(&overSource)
		overSource = strings.Trim(overSource, " ")
		ovrPkgSelected = gps.OverridePackage{
			Name:       overName,
			Constraint: overVersion,
			Source:     overSource,
		}
	} else {
		ovrPkgSelected = ovrPkgs[i-2]
	}
	errInternal = base.AddOverrideToConfig(ovrPkgSelected.Name, ovrPkgSelected.Constraint, ovrPkgSelected.Source,
		ctx.WorkingDir, ctx.Out)
	if errInternal != nil {
		ctx.Err.Println(errInternal)
		return errInternal
	}
	return nil
}
