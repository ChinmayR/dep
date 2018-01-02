package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const monorepoShortHelp = `Resolves dependencies of the target package and adds them to the monorepo.`
const monorepoLongHelp = `Monorepo root is located at the $GOPATH/src directory contains Gopkg.toml manifest, Gopkg.lock lock files
and a vendor directory that define all external dependencies of the monorepo. Newly added package gets it's dependencies resolved
and solved using algorithm similar to dep init. Solution results are represented by the target's manifest and lock are
further merged with manifest and lock of the root package using conflict resolution algorithm described in comments below.`

type monorepoCommand struct {
	target    string
	skipTools bool
	dryRun    bool
	reset     bool
}

type monorepoLocations struct {
	dir      string // Monorepo location.
	cache    string // Temp location where packages can be cached.
	base     string // Location within monorepo folder where objects should be merged.
	repoName string // Name of the repository within domain.
}

var locations monorepoLocations

func (cmd *monorepoCommand) Name() string      { return "monorepo" }
func (cmd *monorepoCommand) Args() string      { return "[root]" }
func (cmd *monorepoCommand) ShortHelp() string { return monorepoShortHelp }
func (cmd *monorepoCommand) LongHelp() string  { return monorepoLongHelp }
func (cmd *monorepoCommand) Hidden() bool      { return true }

func (cmd *monorepoCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&cmd.target, "add", "", "Path where dependency can be cloned from.")
	fs.BoolVar(&cmd.skipTools, "skip-tools", false, "skip importing configuration from other dependency managers.")
	fs.BoolVar(&cmd.dryRun, "dry-run", false, "Run dependency resolution and reporting only but don't write results on disk.")
	fs.BoolVar(&cmd.reset, "reset", false, "Root repo will be reset if this flag is set to true.")
}

func (cmd *monorepoCommand) Run(ctx *dep.Ctx, args []string) error {
	if cmd.target == "" {
		return errors.New("Target package must be specified, please use -add option.")
	}

	err := cmd.detectGoPath(ctx)
	if err != nil {
		return errors.Wrap(err, "cmd.detectGoPath")
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	domain, repoName, err := resolveUrl(cmd.target, sm)
	if err != nil {
		return errors.Wrap(err, "resolveUrl")
	}

	cacheDir, err := ioutil.TempDir("", domain)
	if err != nil {
		return errors.Wrap(err, "ioutil.TempDir")
	}
	defer os.RemoveAll(cacheDir)

	locations = monorepoLocations{dir: ctx.GOPATH, base: filepath.Join("src", domain),
		repoName: repoName, cache: cacheDir}

	if !exists(packageRoot(repoName)) {
		defer os.RemoveAll(locations.cache)
		if ex := exists(filepath.Join(locations.dir, ".git")); !ex {
			initGitRepo(locations.dir, true)
		}
		if cmd.reset {
			resetWorkingDirectory(false)
		}

		cachePackage(cmd.target, repoName)
		mergeIntoMonorepo(repoName)
	} else if ctx.Verbose {
		ctx.Out.Printf("Skipping import phase as repository %v already exists at %v\n", cmd.target, packageRoot(repoName))
	}

	if err := cmd.resolveDependencies(ctx, sm, args); err != nil {
		return err
	}

	// TODO run verification to ensure that there are no orphaned packages left.
	// TODO generate BUCK files.

	return nil
}

/* Dependency resolution is a multi step process:
	1. Read constraints from the monorepo.
	2. Read constraints from the target package that we need to add to the monorepo.
	3. Merge constraints from monorepo to the target prefering version from the monorepo.
   	   This is done to ensure that solution satisfies constraints already defined in the monorepo.
	4. Run dep solver and find a solution that satisfies constraints.
	5. Merge solution back to the monorepo lock and manifest preferring versions that already are in the monorepo.
	6. If dry-run flag is not set, write changes to the lock, manifest and vendor on disk. */
func (cmd *monorepoCommand) resolveDependencies(ctx *dep.Ctx, sm *gps.SourceMgr, args []string) error {
	proj, err := initProject(ctx, packageRoot(locations.repoName))
	if err != nil {
		return err
	}

	root := filepath.Join(ctx.GOPATH, "src")
	rootProj, err := initProject(ctx, root)
	if err != nil {
		return errors.Wrap(err, "initProject")
	}

	ctx.LoadProject()

	if ctx.Verbose {
		ctx.Out.Println("Getting direct dependencies...")
	}

	pkgT, directDeps, err := getDirectDependencies(sm, proj)
	if err != nil {
		return errors.Wrap(err, "getDirectDependencies")
	}
	if ctx.Verbose {
		ctx.Out.Printf("Checked %d directories for packages.\nFound %d direct dependencies.\n", len(pkgT.Packages), len(directDeps))
	}

	rootAnalyzer := newRootAnalyzer(cmd.skipTools, ctx, directDeps, sm)
	// Read Dep manifest if it already exists, otherwise init a new one, converting from other tools.
	proj.Manifest, proj.Lock, err = ctx.ReadManifestAndLock(packageRoot(locations.repoName))
	if err != nil {
		proj.Manifest, proj.Lock, err = rootAnalyzer.InitializeRootManifestAndLock(packageRoot(locations.repoName), proj.ImportRoot)
		if err != nil {
			return err
		}
	}
	copyLock := *proj.Lock // Copy lock before solving. Use this to separate new lock projects from solved lock.
	// Now read manifest and lock for the root project.
	rootProj.Manifest, rootProj.Lock, err = ctx.ReadManifestAndLock(root)
	if err != nil {
		ctx.Err.Println("Unable to read root manifest and lock. Initializing empty ones.")
		rootProj.Manifest = dep.NewManifest()
		rootProj.Lock = &dep.Lock{}
	}

	if err = mergeMetadata(rootProj, proj, ctx); err != nil {
		return errors.Wrap(err, "mergeMetadata: monorepo constraints to target")
	}

	params := gps.SolveParameters{
		RootDir:         packageRoot(locations.repoName),
		RootPackageTree: pkgT,
		Manifest:        proj.Manifest,
		Lock:            proj.Lock,
		ProjectAnalyzer: rootAnalyzer,
	}

	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	}
	if err := ctx.ValidateParams(sm, params); err != nil {
		return err
	}

	s, err := gps.Prepare(params, sm)
	if err != nil || s == nil {
		return errors.Wrap(err, "prepare solver")
	}

	if ctx.Verbose {
		ctx.Out.Println("Solver ready, starting dependency resolution.")
	}

	soln, err := s.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return err
	}

	proj.Lock = dep.LockFromSolution(soln)
	rootAnalyzer.FinalizeRootManifestAndLock(proj.Manifest, proj.Lock, copyLock)

	if err = mergeMetadata(proj, rootProj, ctx); err != nil {
		return errors.Wrap(err, "mergeMetadata: solution constraints back to monorepo.")
	}
	// Since we've just added proj to the monorepo it should no longer be in the manifest and lock.
	deleteFromRootLockAndManifest(rootProj, proj)

	if !cmd.dryRun {
		sw, err := dep.NewSafeWriter(rootProj.Manifest, nil, rootProj.Lock, dep.VendorAlways)
		if err != nil {
			return err
		}

		logger := ctx.Err
		if err := sw.Write(root, sm, false, logger); err != nil {
			return errors.Wrap(err, "safe write of manifest and lock")
		}
	}
	return nil
}

/* Removes project's import root from the root project manifest and lock.
   This is needed when we've just added a package that was referenced from the monorepo.
   It doesn't need to be resolved as now it is available locally. */
func deleteFromRootLockAndManifest(rootProj *dep.Project, proj *dep.Project) {
	delete(rootProj.Manifest.Constraints, proj.ImportRoot)
	// In normal case package import root doesn't contain .git suffix but package can be imported with .git suffix externally.
	// So we will try both permutations and delete them from manifest and lock if present.
	packageNameWithGitSuffix := gps.ProjectRoot(string(proj.ImportRoot) + ".git")
	delete(rootProj.Manifest.Constraints, packageNameWithGitSuffix)
	for idx, rl := range rootProj.Lock.P {
		if rl.Ident().ProjectRoot == proj.ImportRoot || rl.Ident().ProjectRoot == packageNameWithGitSuffix {
			rootProj.Lock.P = append(rootProj.Lock.P[:idx], rootProj.Lock.P[idx+1:]...)
		}
	}
}

/*  Merging is a process of applying constraints from the source project to the target.
Algorithm goes through the list of constraints in the source project and considers following use cases:
	1. ✓ Package is not yet locked in the target project.
	2. ✓ Package is locked in both source and target and versions are matching.
	3. ? There is a version mismatch between source and target.
	3.1. ✓ Constraint version from the target manifest matches version locked in the source.
	3.2. ? Selected version is not allowed by constraint in the root project manifest.
	3.2.1. ✓ Target constraint can be dropped. (e.g. reference to the master branch)
	3.2.2. ✗ Target constraint can't be dropped.
	3.3. ✓ There is no constraint defined in the root project manifest.

✓ - Constraint is accepted and is applied to the target.
? - Further refinement is needed to make a decision.
✗ - Constraint is rejected. Error.

Last step of the merging process is applying updates to the target project manifest based on version from the source.
Based on presence of the package in source and target manifests there can be 4 diffeerent cases:
	1. present:present - need to intersect constraints and update target.
	2. not-present:present - no action required since package is already in the target manifest and is not present in source.
	3. present:not-present - add a new version to the manifest from source.
	4. not-present:not-present - no action as package is not a direct dependency and doesn't need to be in the manifest. */
func mergeMetadata(source *dep.Project, target *dep.Project, ctx *dep.Ctx) error {
	failures := make(map[gps.ProjectRoot]struct {
		sourceLock     string
		targetLock     string
		targetManifest string
	})
	for _, sourceLock := range source.Lock.P {
		alreadyLocked := false                        // True if version is locked in the target package lock.
		projectRoot := sourceLock.Ident().ProjectRoot // Project name.
		for idx, targetLock := range target.Lock.P {
			if targetLock.Ident().ProjectRoot == projectRoot { // Found a package that is present in both source and target.
				if targetLock.Version().Matches(sourceLock.Version()) {
					ctx.Err.Printf("  ✓ %v with version [%v] in the source lock matches constraint [%v] defined in the target lock. Taking [%v].\n",
						sourceLock.Ident(), sourceLock.Version(), targetLock.Version(), sourceLock.Version())
					target.Lock.P[idx].OverrideFrom(sourceLock)
				} else { // We have a potential version conflict.
					ctx.Err.Printf("  ? %v was resolved to [%v] in the source but is already locked to [%v] in the target\n",
						sourceLock.Ident().ProjectRoot, sourceLock.Version(), targetLock.Version())
					foundInManifest := false
					// Check if newly selected version is allowed by constraints in the root manifest.
					for pkg, targetManifest := range target.Manifest.Constraints {
						if pkg == projectRoot { // Dependency name matches.
							if targetManifest.Constraint.Matches(sourceLock.Version()) {
								// Take version from the target package.
								ctx.Err.Printf("    ✓ Version [%v] from the source accepted by constraint [%v] in the target manifest.\n",
									sourceLock.Version(), targetManifest.Constraint)
								target.Lock.P[idx].OverrideFrom(sourceLock)
							} else {
								if targetManifest.Constraint.String() == "master" {
									ctx.Err.Printf("    ✓ Version [%v] was rejected by master constraint, but master constraint can be dropped.\n",
										sourceLock.Version(), targetManifest.Constraint)
									target.Lock.P[idx].OverrideFrom(sourceLock)
								} else {
									ctx.Err.Printf("    ✗ Version [%v] was rejected by constraint [%v] in the target manifest.\n",
										sourceLock.Version(), targetManifest.Constraint)
									failures[pkg] = struct {
										sourceLock     string
										targetLock     string
										targetManifest string
									}{
										sourceLock.Version().String(), targetLock.Version().String(), targetManifest.Constraint.String(),
									}
								}
							}
							foundInManifest = true
						}
					}
					if !foundInManifest {
						ctx.Err.Printf("    ✓ Constraint was not found in the root manifest. Taking [%v].\n", sourceLock.Version())
						target.Lock.P[idx].OverrideFrom(sourceLock)
					}
				}
				alreadyLocked = true
				break // No need to search further, we've already found a match.
			}
		}
		if !alreadyLocked {
			if ctx.Verbose {
				ctx.Err.Printf("  ✓ %v was resolved to [%v] in the target and was not locked yet in the root.\n", sourceLock.Ident(), sourceLock.Version())
			}
			target.Lock.P = append(target.Lock.P, sourceLock)
		}

		sourceManifest := findInManifest(source.Manifest, projectRoot)
		targetManifest := findInManifest(target.Manifest, projectRoot)
		switch {
		case sourceManifest != nil && targetManifest != nil:
			intersection := targetManifest.Constraint.Intersect(sourceManifest.Constraint)
			ctx.Err.Printf("    Merging source manifest constraint [%v] with target manifest constraint [%v]. Result is [%v].",
				targetManifest.Constraint, sourceManifest.Constraint, intersection)
			sourceManifest.Constraint = intersection
			target.Manifest.Constraints[projectRoot] = *sourceManifest
		case sourceManifest == nil && targetManifest != nil:
			ctx.Err.Printf("    Keeping manifest constraint [%v] as source doesn't have dependency defined in it's manifest.", targetManifest.Constraint)
		case sourceManifest != nil && targetManifest == nil:
			ctx.Err.Printf("    Taking source's manifest constraint [%v] as target manifest didn't have a record.", sourceManifest.Constraint)
			target.Manifest.Constraints[projectRoot] = *sourceManifest
		default: // Both null.
			ctx.Err.Printf("    Package is not present in source and target manifests, ignoring it.")
		}
	}
	if len(failures) > 0 {
		ctx.Err.Println("Failures summary:")
		for pkg, details := range failures {
			ctx.Err.Printf("%v with source lock [%v] didn't match [%v] in the target lock with [%v] manifest constraint.",
				pkg, details.sourceLock, details.targetLock, details.targetManifest)
		}
		return errors.New("failed to merge metadata.", )
	}
	return nil
}

func findInManifest(manifest *dep.Manifest, projectRoot gps.ProjectRoot) *gps.ProjectProperties {
	for pkg, mc := range manifest.Constraints {
		if pkg == projectRoot {
			return &mc
		}
	}
	return nil
}

func (cmd *monorepoCommand) detectGoPath(ctx *dep.Ctx) error {
	p := new(dep.Project)
	err := p.SetRoot(ctx.WorkingDir)
	if err != nil {
		return errors.Wrap(err, "NewProject")
	}

	ctx.GOPATH, err = ctx.DetectProjectGOPATH(p)
	return err
}

func initProject(ctx *dep.Ctx, path string) (*dep.Project, error) {
	proj := new(dep.Project)
	if err := proj.SetRoot(path); err != nil {
		return nil, errors.Wrap(err, "NewProject")
	}

	if path == filepath.Join(ctx.GOPATH, "src") { // ImportForAbs doesn't allow importing GOPATH/src.
		proj.ImportRoot = gps.ProjectRoot("")
	} else {
		ip, err := ctx.ImportForAbs(path)
		if err != nil {
			return nil, errors.Wrap(err, "root project import")
		}
		proj.ImportRoot = gps.ProjectRoot(ip)
	}

	return proj, nil
}

// Creates a directory on the filesystem if needed and initializes git repository, doing initial commit.
func initGitRepo(path string, initialCommit bool) {
	createDir(path)
	run(exec.Command("git", "-C", path, "init"), true)
	// Prevent "inexact rename detection was skipped due to too many files" warnings from git.
	run(exec.Command("git", "-C", path, "config", "merge.renameLimit", "100000"), false)
	if initialCommit {
		run(exec.Command("touch", path+"/.monorepo"), true)
		run(exec.Command("git", "-C", path, "add", ".monorepo"), true)
		run(exec.Command("git", "-C", path, "commit", "-m", "Initial commit"), true)
	}
}

// Caches package in the temporary location. Emits a merge signal when done.
// If update flag is set to false in settings then it always updates packages, otherwise skips those that were checked out already.
func cachePackage(fetchUrl string, repoName string) string {
	cacheLocation := cacheLocation(repoName)
	initGitRepo(cacheLocation, false)
	fetch(cacheLocation, repoName, fetchUrl)
	run(exec.Command("git", "-C", cacheLocation, "merge", "--allow-unrelated-histories", filepath.Join(repoName, "master")), false)
	deleteSubmodules(repoName)
	deleteIfExists(repoName, "vendor")
	deleteIfExists(repoName, "oss_repos")
	deleteIfExists(repoName, "Godeps/_workspace")
	return repoName
}

func resolveUrl(fetchUrl string, sm *gps.SourceMgr) (domain string, repoName string, err error) {
	urls, err := sm.SourceURLsForPath(fetchUrl)
	if err != nil {
		return
	}
	url := urls[0]
	domain = url.Host
	repoName = url.Path
	if strings.HasPrefix(repoName, "/") { // Remove leading slash if it exists.
		repoName = repoName[1:]
	}
	return
}

// Deletes all submodules from the cached package.
func deleteSubmodules(repoName string) {
	cacheLocation := cacheLocation(repoName)
	submodules := run(exec.Command("git", "-C", cacheLocation, "config", "--file", ".gitmodules", "--get-regexp", "path"), true)
	lines := strings.Split(submodules, "\n")
	deleted := false
	for _, line := range lines {
		if line == "" {
			continue
		}
		submoduleName := strings.Split(line, " ")[1]
		run(exec.Command("git", "-C", cacheLocation, "submodule", "deinit", submoduleName), true)
		run(exec.Command("git", "-C", cacheLocation, "rm", "-r", submoduleName), true)
		deleted = true
	}
	if deleted {
		run(exec.Command("git", "-C", cacheLocation, "rm", "-f", ".gitmodules"), true)
		run(exec.Command("git", "-C", cacheLocation, "commit", "-m", "Deleted submodules"), true)
	}
}

func deleteIfExists(repoName string, path string) {
	cacheLocation := cacheLocation(repoName)
	if exists(filepath.Join(cacheLocation, path)) {
		run(exec.Command("git", "-C", cacheLocation, "rm", "-rf", path), false)
		run(exec.Command("git", "-C", cacheLocation, "commit", "-m", "Deleted "+path), false)
	}
}

// Merges remote into monorepo by performing fetch, merge and read-tree operations.
// You can read more about the method here https://help.github.com/articles/about-git-subtree-merges
func mergeIntoMonorepo(repoName string) {
	srcDir := packageRoot(repoName) // Directory where package foo/bar will be checked out. For example /home/user/monorepo/code.uber.internal/foo/bar
	cacheLocation := cacheLocation(repoName)
	createDir(srcDir)
	fetch(locations.dir, repoName, cacheLocation)
	run(exec.Command("git", "-C", locations.dir, "merge", "-s", "ours", "--no-commit", "--allow-unrelated-histories", filepath.Join(repoName, "master")), false)
	run(exec.Command("git", "-C", locations.dir, "read-tree", fmt.Sprintf("--prefix=%v", filepath.Join(locations.base, repoName)), "-u", filepath.Join(repoName, "master")), false)
	run(exec.Command("git", "-C", locations.dir, "commit", "-m", fmt.Sprintf("Add %v to monorepo.", repoName)), false)
	run(exec.Command("git", "-C", locations.dir, "remote", "remove", repoName), false)
	// Delete remote once everything is merged.
	fmt.Println("Successfully checked out", repoName, "into", srcDir)
}

// Adds remote and fetches content given url and repo location.
// Repo location is used as a name of remote.
func fetch(path string, repo string, fetchUrl string) {
	run(exec.Command("git", "-C", path, "remote", "remove", repo), true)
	run(exec.Command("git", "-C", path, "remote", "add", repo, fetchUrl), false)
	run(exec.Command("git", "-C", path, "fetch", repo, "master"), false)
}

// This function removes all untracked and uncommitted files from the git directory. Returns total time it took to run operation.
func resetWorkingDirectory(optional bool) {
	run(exec.Command("git", "-C", locations.dir, "reset", "--hard", "HEAD"), optional)
	run(exec.Command("git", "-C", locations.dir, "clean", "-df"), optional)
}

// Location where cached artifact is located.
func cacheLocation(repoName string) string {
	return filepath.Join(locations.cache, locations.base, repoName)
}

// Returns package location in the monorepo.
func packageRoot(repoName string) string {
	return filepath.Join(locations.dir, locations.base, repoName)
}

// Creates directory and all subdirectories if needed.
func createDir(path string) {
	if e := os.MkdirAll(path, os.ModePerm); e != nil {
		fmt.Println(e)
		os.Exit(1)
	}
}

// Returns true if directory or file exists at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

// Execute a command, returning output value. Boolean flag can be used to control panic behavior. If set to true panics are suppressed.
func run(cmd *exec.Cmd, safely bool) string {
	var out bytes.Buffer
	var err bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &err
	e := cmd.Run()
	if e != nil && !safely {
		fmt.Printf("Got error: %v while running %v.\n", err.String(), cmd.Args)
		panic(e)
	}
	return out.String()
}
