// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate ./mkdoc.sh

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"

	"strings"
	"text/tabwriter"
	"time"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/uber"
)

var (
	successExitCode = 0
	errorExitCode   = 1
)

type command interface {
	Name() string           // "foobar"
	Args() string           // "<baz> [quux...]"
	ShortHelp() string      // "Foo the first bar"
	LongHelp() string       // "Foo the first bar meeting the following conditions..."
	Register(*flag.FlagSet) // command-specific flags
	Hidden() bool           // indicates whether the command should be hidden from help output
	Run(*dep.Ctx, []string) error
}

func main() {
	p := &profile{}
	flag.StringVar(&p.cpuProfile, "cpuprofile", "", "Writes a CPU profile to the specified file before exiting.")
	flag.StringVar(&p.memProfile, "memprofile", "", "Writes a memory profile to the specified file before exiting.")
	flag.IntVar(&p.memProfileRate, "memprofilerate", 0, "Enable more precise memory profiles by setting runtime.MemProfileRate.")
	flag.StringVar(&p.mutexProfile, "mutexprofile", "", "Writes a mutex profile to the specified file before exiting.")
	flag.IntVar(&p.mutexProfileFraction, "mutexprofilefraction", 0, "Enable more precise mutex profiles by runtime.SetMutexProfileFraction.")
	flag.Parse()

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get working directory", err)
		os.Exit(1)
	}

	args := append([]string{os.Args[0]}, flag.Args()...)
	c := &Config{
		Args:       args,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		WorkingDir: wd,
		Env:        os.Environ(),
	}

	if err := p.start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to profile: %v\n", err)
		os.Exit(1)
	}
	exit := c.Run()
	if err := p.finish(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to finish the profile: %v\n", err)
		os.Exit(1)
	}
	os.Exit(exit)
}

// A Config specifies a full configuration for a dep execution.
type Config struct {
	WorkingDir     string    // Where to execute
	Args           []string  // Command-line arguments, starting with the program name.
	Env            []string  // Environment variables
	Stdout, Stderr io.Writer // Log output
}

// Run executes a configuration and returns an exit code.
func (c *Config) Run() int {
	// Build the list of available commands.
	commands := [...]command{
		&initCommand{},
		&monorepoCommand{},
		&statusCommand{},
		&ensureCommand{},
		&pruneCommand{},
		&hashinCommand{},
		&versionCommand{},
		&bootConfigCommand{},
		&cacheClearCommand{},
		&syncManifestCommand{},
	}

	examples := [...][2]string{
		{
			"dep init",
			"set up a new project",
		},
		{
			"dep ensure",
			"install the project's dependencies",
		},
		{
			"dep ensure -update",
			"update the locked versions of all dependencies",
		},
		{
			"dep ensure -add github.com/pkg/errors",
			"add a dependency to the project",
		},
		{
			"dep bootConfig",
			"bootstrap the uber specific default config",
		},
		{
			"dep cc",
			"clear the dep cache at $HOME/.dep-cache/pkg/dep",
		},
	}

	outWriter := io.MultiWriter(c.Stdout, uber.LogFile)
	errWriter := io.MultiWriter(c.Stderr, uber.LogFile)
	outLogger := log.New(outWriter, uber.DEP_PREFIX, 0)
	errLogger := log.New(errWriter, uber.DEP_PREFIX, 0)

	usage := func(w io.Writer) {
		fmt.Fprintln(w, "Dep is a tool for managing dependencies for Go projects")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Usage: \"dep [command]\"")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Commands:")
		fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, cmd := range commands {
			if !cmd.Hidden() {
				fmt.Fprintf(tw, "\t%s\t%s\n", cmd.Name(), cmd.ShortHelp())
			}
		}
		tw.Flush()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		for _, example := range examples {
			fmt.Fprintf(tw, "\t%s\t%s\n", example[0], example[1])
		}
		tw.Flush()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Use \"dep help [command]\" for more information about a command.")
	}

	cmdName, printCommandHelp, exit := parseArgs(c.Args)
	if exit {
		usage(errWriter)
		return errorExitCode
	}

	uber.UberLogger.Println("DEP VERSION: " + uber.DEP_VERSION)
	uber.UberLogger.Printf("RUNNING COMMAND: %v\n", c.Args)
	uber.UberLogger.Println("RUN ID: " + uber.RunId)
	uber.UberLogger.Println("Log stored at: " + uber.LogPath)

	versionInfo, err := uber.IsLatestVersion(uber.DEP_VERSION)
	if err != nil {
		uber.DebugLogger.Printf("Error checking if current version is latest: %v", err)
	}
	if versionInfo.IsLatest {
		uber.UberLogger.Printf("Your dep version %v is latest", uber.DEP_VERSION)
	} else {
		uber.UberLogger.Println("*******************")
		uber.UberLogger.Printf("Your dep version is %v but there is a newer version %v available\n", uber.DEP_VERSION, versionInfo.LatestVersion)
		uber.UberLogger.Println("Please update Dep by updating to latest go-build (make update-gobuild)...")
		uber.UberLogger.Println("*******************")
		// sleep for 5 seconds to allow the user to read the above message and also as an incentive to update
		time.Sleep(5 * time.Second)
	}

	_, _, err = new(uber.CommandExecutor).ExecCommand("git", time.Duration(1*time.Minute),
		false, nil, "config", "--global", "--unset", "url.ssh://git@github.com/uber/.insteadof", "https://github.com/uber/")
	if err != nil {
		uber.UberLogger.Println("If dep hangs, run \"ssh-add\" and try again")
	} else {
		defer func() {
			new(uber.CommandExecutor).ExecCommand("git", time.Duration(1*time.Minute),
				false, nil, "config", "--global", "url.ssh://git@github.com/uber/.insteadof", "https://github.com/uber/")
		}()
	}

	defer func() int {
		if r := recover(); r != nil {
			errLogger.Printf("PANIC: EMAIL dep-support-group@uber.com WITH THE LOG FROM %s\n", uber.LogPath)
			errLogger.Printf("[PANIC] %s\n\n", r)
			errLogger.Printf("%s\n", debug.Stack())
			return errorExitCode
		}
		return successExitCode
	}()

	// 'dep help documentation' generates doc.go.
	if printCommandHelp && cmdName == "documentation" {
		fmt.Println("// Copyright 2017 The Go Authors. All rights reserved.")
		fmt.Println("// Use of this source code is governed by a BSD-style")
		fmt.Println("// license that can be found in the LICENSE file.")
		fmt.Println()
		fmt.Println("// DO NOT EDIT THIS FILE. GENERATED BY mkdoc.sh.")
		fmt.Println("// Edit the documentation in other files and rerun mkdoc.sh to generate this one.")
		fmt.Println()

		var cw io.Writer = &commentWriter{W: outWriter}
		usage(cw)
		for _, cmd := range commands {
			if !cmd.Hidden() {
				fmt.Fprintln(cw)
				short := cmd.ShortHelp()
				fmt.Fprintln(cw, short)
				fmt.Fprintln(cw)
				fmt.Fprintln(cw, "Usage:")
				fmt.Fprintln(cw)
				fmt.Fprintln(cw, "", cmd.Name(), cmd.Args())
				if long := cmd.LongHelp(); long != short {
					fmt.Fprintln(cw, long)
				}
			}
		}

		fmt.Println("//")
		fmt.Println("package main")
		return successExitCode
	}

	for _, cmd := range commands {
		if cmd.Name() == cmdName {
			// Build flag set with global flags in there.
			flags := flag.NewFlagSet(cmdName, flag.ContinueOnError)
			flags.SetOutput(errWriter)
			verbose := flags.Bool("v", true, "enable verbose logging")

			// Register the subcommand flags in there, too.
			cmd.Register(flags)

			// Override the usage text to something nicer.
			resetUsage(errLogger, flags, cmdName, cmd.Args(), cmd.LongHelp())

			if printCommandHelp {
				flags.Usage()
				return errorExitCode
			}

			// Parse the flags the user gave us.
			// flag package automatically prints usage and error message in err != nil
			// or if '-h' flag provided
			if err := flags.Parse(c.Args[2:]); err != nil {
				return errorExitCode
			}

			// Cachedir is loaded from env if present. `$GOPATH/pkg/dep` is used as the
			// default cache location.
			cachedir := getEnv(c.Env, "DEPCACHEDIR")
			if cachedir != "" {
				if err := fs.EnsureDir(cachedir, 0777); err != nil {
					errLogger.Printf(
						"dep: $DEPCACHEDIR set to an invalid or inaccessible path: %q\n", cachedir,
					)
					errLogger.Printf("dep: failed to ensure cache directory: %v\n", err)
					return errorExitCode
				}
			}

			// Set up dep context.
			ctx := &dep.Ctx{
				Out:            outLogger,
				Err:            errLogger,
				Verbose:        *verbose,
				DisableLocking: getEnv(c.Env, "DEPNOLOCK") != "",
				Cachedir:       cachedir,
			}

			GOPATHS := filepath.SplitList(getEnv(c.Env, "GOPATH"))
			ctx.SetPaths(c.WorkingDir, GOPATHS...)

			if cmdName != "cc" {
				doesCacheNeedToBeCleared, cacheClearedVersion, err := uber.DoesCacheNeedToBeCleared(uber.LATEST_CACHE_ALLOWED_VERSION)
				if err != nil {
					uber.DebugLogger.Printf("Error checking if cache needs to be cleared: %v", err)
				}
				if doesCacheNeedToBeCleared {
					uber.UberLogger.Printf("Your dep cache is written by a version %v which is less than the minimum allowed version of %v\n", cacheClearedVersion, uber.LATEST_CACHE_ALLOWED_VERSION)
					uber.UberLogger.Printf("Clearing your cache by running \"dep cc\" before continuing\n")
					if err := (&cacheClearCommand{}).Run(ctx, flags.Args()); err != nil {
						errLogger.Printf("%v\n", err)
						return errorExitCode
					}
				}
			}

			// Run the command with the post-flag-processing args.
			if err := cmd.Run(ctx, flags.Args()); err != nil {
				errLogger.Printf("%v\n", err)
				return errorExitCode
			}

			// Easy peasy livin' breezy.
			return successExitCode
		}
	}

	errLogger.Printf("dep: %s: no such command\n", cmdName)
	usage(errWriter)
	return errorExitCode
}

func resetUsage(logger *log.Logger, fs *flag.FlagSet, name, args, longHelp string) {
	var (
		hasFlags   bool
		flagBlock  bytes.Buffer
		flagWriter = tabwriter.NewWriter(&flagBlock, 0, 4, 2, ' ', 0)
	)
	fs.VisitAll(func(f *flag.Flag) {
		hasFlags = true
		// Default-empty string vars should read "(default: <none>)"
		// rather than the comparatively ugly "(default: )".
		defValue := f.DefValue
		if defValue == "" {
			defValue = "<none>"
		}
		fmt.Fprintf(flagWriter, "\t-%s\t%s (default: %s)\n", f.Name, f.Usage, defValue)
	})
	flagWriter.Flush()
	fs.Usage = func() {
		logger.Printf("Usage: dep %s %s\n", name, args)
		logger.Println()
		logger.Println(strings.TrimSpace(longHelp))
		logger.Println()
		if hasFlags {
			logger.Println("Flags:")
			logger.Println()
			logger.Println(flagBlock.String())
		}
	}
}

// parseArgs determines the name of the dep command and whether the user asked for
// help to be printed.
func parseArgs(args []string) (cmdName string, printCmdUsage bool, exit bool) {
	isHelpArg := func() bool {
		return strings.Contains(strings.ToLower(args[1]), "help") || strings.ToLower(args[1]) == "-h"
	}

	switch len(args) {
	case 0, 1:
		exit = true
	case 2:
		if isHelpArg() {
			exit = true
		} else {
			cmdName = args[1]
		}
	default:
		if isHelpArg() {
			cmdName = args[2]
			printCmdUsage = true
		} else {
			cmdName = args[1]
		}
	}
	return cmdName, printCmdUsage, exit
}

// getEnv returns the last instance of an environment variable.
func getEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		v := env[i]
		kv := strings.SplitN(v, "=", 2)
		if kv[0] == key {
			if len(kv) > 1 {
				return kv[1]
			}
			return ""
		}
	}
	return ""
}

// commentWriter writes a Go comment to the underlying io.Writer,
// using line comment form (//).
//
// Copied from cmd/go/internal/help/help.go.
type commentWriter struct {
	W            io.Writer
	wroteSlashes bool // Wrote "//" at the beginning of the current line.
}

func (c *commentWriter) Write(p []byte) (int, error) {
	var n int
	for i, b := range p {
		if !c.wroteSlashes {
			s := "//"
			if b != '\n' {
				s = "// "
			}
			if _, err := io.WriteString(c.W, s); err != nil {
				return n, err
			}
			c.wroteSlashes = true
		}
		n0, err := c.W.Write(p[i : i+1])
		n += n0
		if err != nil {
			return n, err
		}
		if b == '\n' {
			c.wroteSlashes = false
		}
	}
	return len(p), nil
}

type profile struct {
	cpuProfile string

	memProfile     string
	memProfileRate int

	mutexProfile         string
	mutexProfileFraction int

	// TODO(jbd): Add block profile and -trace.

	f *os.File // file to write the profiling output to
}

func (p *profile) start() error {
	switch {
	case p.cpuProfile != "":
		if err := p.createOutput(p.cpuProfile); err != nil {
			return err
		}
		return pprof.StartCPUProfile(p.f)
	case p.memProfile != "":
		if p.memProfileRate > 0 {
			runtime.MemProfileRate = p.memProfileRate
		}
		return p.createOutput(p.memProfile)
	case p.mutexProfile != "":
		if p.mutexProfileFraction > 0 {
			runtime.SetMutexProfileFraction(p.mutexProfileFraction)
		}
		return p.createOutput(p.mutexProfile)
	}
	return nil
}

func (p *profile) finish() error {
	if p.f == nil {
		return nil
	}
	switch {
	case p.cpuProfile != "":
		pprof.StopCPUProfile()
	case p.memProfile != "":
		if err := pprof.WriteHeapProfile(p.f); err != nil {
			return err
		}
	case p.mutexProfile != "":
		if err := pprof.Lookup("mutex").WriteTo(p.f, 2); err != nil {
			return err
		}
	}
	return p.f.Close()
}

func (p *profile) createOutput(name string) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	p.f = f
	return nil
}
