package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v2"
)

// absolutePath may be used in go-flags structs for paths that must be
// resolved to absolute relative to the current directory at parse time.
//
// Symlinks are evaluated away as well.
type absolutePath string

func (p *absolutePath) UnmarshalFlag(value string) error {
	if value == "" {
		*p = ""
		return nil
	}

	newValue, err := filepath.EvalSymlinks(value)
	if err != nil {
		return fmt.Errorf("could not resolve symlink at %q: %v", value, err)
	}
	value = newValue

	newValue, err = filepath.Abs(value)
	if err != nil {
		return fmt.Errorf("could not determine absolute path of %q: %v", value, err)
	}
	value = newValue

	*p = absolutePath(value)
	return nil
}

func (p absolutePath) String() string {
	return string(p)
}

type options struct {
	Glide       absolutePath `long:"glide" short:"g" description:"Path to the glide executable."`
	NoColor     bool         `long:"no-color" description:"Turn off colored output."`
	BinDir      absolutePath `long:"bin" short:"d" required:"true" value-name:"DIR" description:"Directory into which executables will be saved."`
	ImportPaths []importPath `long:"exe" short:"x" required:"true" value-name:"IMPORTPATH" description:"One or more import paths to executables which must be available."`
	Exec        struct {
		Command string   `required:"true" positional-arg-name:"CMD" description:"Command to execute."`
		Args    []string `positional-arg-name:"ARGS" description:"Zero or more arguments to pass to the command."`
	} `positional-args:"true" required:"true"`
}

type importPath string

func (i importPath) Base() string {
	return filepath.Base(string(i))
}

// Contains returns true if the given import path is equal to or a child of
// this import path.
func (i importPath) Contains(other importPath) bool {
	return i == other || strings.HasPrefix(string(other), string(i)+"/")
}

type dependency struct {
	ImportPath importPath
	Version    string
}

type project struct {
	RootDirectory string
	// TODO(abg): Detect import path for the project itself

	dependencies []*dependency
}

type glideLock struct {
	Imports []glideImport `yaml:"imports"`
}

type glideImport struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

func newProject(root string) *project {
	return &project{RootDirectory: root}
}

func (p *project) Dependencies() ([]*dependency, error) {
	if p.dependencies != nil {
		return p.dependencies, nil
	}

	lockFile := filepath.Join(p.RootDirectory, "glide.lock")
	contents, err := ioutil.ReadFile(lockFile)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %v", lockFile, err)
	}

	var lock glideLock
	if err := yaml.Unmarshal(contents, &lock); err != nil {
		return nil, fmt.Errorf("could not parse %q: %v", lockFile, err)
	}

	deps := make([]*dependency, 0, len(lock.Imports))
	for _, imp := range lock.Imports {
		dep := &dependency{ImportPath: importPath(imp.Name), Version: imp.Version}
		deps = append(deps, dep)
	}
	p.dependencies = deps
	return deps, nil
}

// GetDependency returns the dependency that covers the given import path.
func (p *project) GetDependency(ip importPath) (*dependency, error) {
	deps, err := p.Dependencies()
	if err != nil {
		return nil, err
	}

	for _, d := range deps {
		if d.ImportPath.Contains(ip) {
			return d, nil
		}
	}

	return nil, fmt.Errorf("%q does not depend on %q", p.RootDirectory, ip)
}

func findProject(startDir string) (*project, error) {
	root, ok := findUp(startDir, "glide.lock")
	root = filepath.Dir(root)
	if ok {
		return newProject(root), nil
	}

	return nil, fmt.Errorf(
		"could not find project root in %q or any of its parent directories", startDir)
}

func main() {
	log.SetFlags(0)

	var opts options
	parser := flags.NewParser(&opts, flags.Default|flags.PassAfterNonOption|flags.IgnoreUnknown)
	parser.Name = "glide exec"

	if _, err := parser.Parse(); err != nil {
		return // message already printed by go-flags
	}

	if s, err := os.Stat(opts.BinDir.String()); err != nil {
		log.Fatalf("Directory %v does not exist: %v", opts.BinDir, err)
	} else if !s.IsDir() {
		log.Fatalf("%q is not a directory: %v", opts.BinDir, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Could not determine current working directory: %v", err)
	}

	proj, err := findProject(cwd)
	if err != nil {
		log.Fatal(err)
	}

	allExecutables := make(map[string]struct{})
	toBuild := make([]executable, 0, len(opts.ImportPaths))
	for _, ip := range opts.ImportPaths {
		allExecutables[ip.Base()] = struct{}{}

		dep, err := proj.GetDependency(ip)
		if err != nil {
			log.Fatalf("Project %q does not depend on %q",
				proj.RootDirectory, string(ip))
		}

		currentVersion, err := builtVersion(opts.BinDir.String(), ip)
		if err != nil {
			log.Fatal(err)
		}

		if currentVersion == dep.Version {
			continue
		}

		toBuild = append(toBuild, executable{
			ImportPath: ip,
			Version:    dep.Version,
		})
	}

	if len(toBuild) > 0 {
		if err := build(opts, proj, toBuild); err != nil {
			log.Fatal(err)
		}
	}

	var foundPath bool
	env := os.Environ()
	for i, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			continue
		}

		foundPath = true
		env[i] = "PATH=" + opts.BinDir.String() + string(os.PathListSeparator) + e[5:]
	}

	if !foundPath {
		env = append(env, "PATH="+opts.BinDir.String())
	}

	cmdPath := opts.Exec.Command
	if _, ok := allExecutables[cmdPath]; ok {
		cmdPath = filepath.Join(opts.BinDir.String(), cmdPath)
	} else {
		cmdPath, err = exec.LookPath(cmdPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	cmd := exec.Command(cmdPath, opts.Exec.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

type executable struct {
	ImportPath importPath
	Version    string
}

func build(opts options, proj *project, exes []executable) error {
	binDir := opts.BinDir.String()
	vendorDir := filepath.Join(proj.RootDirectory, "vendor")
	if _, err := os.Stat(vendorDir); err != nil {
		return fmt.Errorf(
			"%q does not have a vendor directory: %v", proj.RootDirectory, err)
	}

	return withTemporaryGoPath(func(goPath string) error {
		// Replace the src directory with a symlink to the vendor directory
		srcDir := filepath.Join(goPath, "src")
		if err := os.RemoveAll(srcDir); err != nil {
			return err
		}

		if err := os.Symlink(vendorDir, srcDir); err != nil {
			return err
		}
		defer os.Remove(srcDir)

		builder := &builder{
			Glide:   string(opts.Glide),
			NoColor: opts.NoColor,
			GoPath:  goPath,
		}

		for _, exe := range exes {
			exeName := exe.ImportPath.Base()
			exePath := filepath.Join(binDir, exeName)
			versionPath := filepath.Join(binDir, "."+exeName+"-version")

			if err := builder.Build(string(exe.ImportPath), exePath); err != nil {
				return fmt.Errorf("could not build %q: %v", exe.ImportPath, err)
			}

			if err := ioutil.WriteFile(versionPath, []byte(exe.Version), 0644); err != nil {
				return fmt.Errorf(
					"could not write version %q to %q: %v", exe.Version, versionPath, err)
			}
		}

		return nil
	})
}

// withTemporaryGoPath set up a temporary directory that can act as a valid
// GOPATH and call the given function with its path.
//
// The directory is cleaned up after the function exits.
func withTemporaryGoPath(f func(string) error) error {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "glide-exec-build")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	for _, d := range []string{"src", "pkg", "bin"} {
		if err := os.Mkdir(filepath.Join(tmpDir, d), 0755); err != nil {
			return err
		}
	}

	return f(tmpDir)
}

// findUp looks for any of the given files in the given directory and all its
// parent directories.
//
// The full path to the first found file is returned, or false is returned if
// none of the files were found in the tree.
func findUp(dir string, files ...string) (string, bool) {
	for {
		for _, file := range files {
			path := filepath.Join(dir, file)
			if _, err := os.Stat(path); err == nil {
				return path, true
			}
		}

		newDir := filepath.Dir(dir)
		if newDir == dir {
			return "", false
		}
		dir = newDir
	}
}

// builtVersion returns the currently built version of the executable (stored
// in the given directory) or an empty string if it has not yet been built.
func builtVersion(binDir string, ip importPath) (string, error) {
	name := ip.Base()
	versionFile := filepath.Join(binDir, "."+name+"-version")

	if _, err := os.Stat(versionFile); err != nil {
		return "", nil
	}

	version, err := ioutil.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("could not read version file: %v", err)
	}
	return strings.TrimSpace(string(version)), nil
}

type builder struct {
	Glide   string // path to glide or empty
	NoColor bool   // whether --no-color should be passed to glide
	GoPath  string // GOPATH containing source code for all executables
}

func (b *builder) glide(args ...string) *exec.Cmd {
	cmd := b.Glide
	if cmd == "" {
		cmd = "glide"
	}

	newArgs := make([]string, 0, len(args)+1)
	if b.NoColor {
		newArgs = append(newArgs, "--no-color")
	}
	newArgs = append(newArgs, args...)

	return exec.Command(cmd, newArgs...)
}

func (b *builder) env() []string {
	// Set the GOPATH
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GOPATH=") {
			env = append(env, e)
		}
	}
	env = append(env, "GOPATH="+b.GoPath)
	return env
}

// Build the executable at the given import path and save it to the given
// destination.
func (b *builder) Build(importPath, dest string) error {
	src := filepath.Join(b.GoPath, "src", importPath)
	if glideLock, ok := findUp(src, "glide.lock"); ok {
		// The executable belongs to a package that has its own glide.lock. We
		// should install its dependencies to build it.
		root := filepath.Dir(glideLock)

		cmd := b.glide("install")
		cmd.Env = b.env()
		cmd.Dir = root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install dependencies for %q: %v", importPath, err)
		}

		// If we successfully installed vendored dependencies, make sure
		// we clean up after we are done.
		defer func() {
			if err := os.RemoveAll(filepath.Join(root, "vendor")); err != nil {
				log.Printf(
					"Failed to delete vendored dependencies for %q: %v", importPath, err)
			}
		}()
	}

	cmd := exec.Command("go", "build", "-o", dest)
	cmd.Env = b.env()
	cmd.Dir = src
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
