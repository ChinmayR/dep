// Package main provides a utility to help go-build use the protoc tool.
package main

import (
	"errors"
	"flag"
	"log"
	"strings"

	"code.uber.internal/infra/prototools.git/internal/protocutil"
)

var flagOptions = &options{}

func init() {
	flag.StringVar(&flagOptions.ProjectRoot, "project-root", "", "The golang import path of the project root")
	flag.StringVar(&flagOptions.InputDirPath, "input-dir", "", "The input directory")
	flag.StringVar(&flagOptions.OutputDirPath, "output-dir", "", "The output directory")
	flag.Var(&flagOptions.ExcludePatterns, "exclude", "Proto files to exclude relative to the import directory")
	flag.Var(&flagOptions.ExtraIncludes, "extra-include", "Directories to include with -I along with the input directory")
	flag.StringVar(&flagOptions.GoPlugin, "go-plugin", "gogoslick", "The golang protoc plugin to use")
	flag.BoolVar(&flagOptions.NoGRPC, "no-grpc", false, "Do not compile gRPC stubs when using the golang protoc plugin")
	flag.Var(&flagOptions.ExtraGoPlugins, "extra-go-plugin", "Use an additional golang protoc plugin, such as yarpc-go or grpc-gateway")
	flag.StringVar(&flagOptions.ProtocPath, "protoc-path", "", "The path to protoc, use protoc on PATH by default")
	flag.BoolVar(&flagOptions.Verbose, "verbose", false, "Print out extra information")
}

type options struct {
	ProjectRoot     string
	InputDirPath    string
	OutputDirPath   string
	ExcludePatterns stringSlice
	ExtraIncludes   stringSlice
	GoPlugin        string
	NoGRPC          bool
	ExtraGoPlugins  stringSlice
	ProtocPath      string
	Verbose         bool
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("")
	options, err := getFlagOptions()
	if err != nil {
		log.Fatal(err)
	}
	if err := do(options); err != nil {
		log.Fatal(err)
	}
}

func getFlagOptions() (*options, error) {
	flag.Parse()
	if err := validateOptions(flagOptions); err != nil {
		flag.Usage()
		return nil, err
	}
	return flagOptions, nil
}

func validateOptions(options *options) error {
	if options.ProjectRoot == "" {
		return errors.New("--project-root required")
	}
	if options.InputDirPath == "" {
		return errors.New("--input-dir required")
	}
	if options.OutputDirPath == "" {
		return errors.New("--output-dir required")
	}
	return nil
}

func do(options *options) error {
	var builderOptions []protocutil.BuilderOption
	if options.GoPlugin != "" {
		builderOptions = append(builderOptions, protocutil.WithGoPlugin(options.GoPlugin))
	}
	if options.NoGRPC {
		builderOptions = append(builderOptions, protocutil.WithAddGRPC(false))
	}
	for _, extraGoPlugin := range options.ExtraGoPlugins {
		builderOptions = append(builderOptions, protocutil.WithExtraGoPlugin(extraGoPlugin))
	}
	if options.Verbose {
		builderOptions = append(builderOptions, protocutil.WithLogFunc(log.Printf))
	}
	if options.ProtocPath != "" {
		builderOptions = append(builderOptions, protocutil.WithProtocPath(options.ProtocPath))
	}
	var buildOptions []protocutil.BuildOption
	for _, excludePattern := range options.ExcludePatterns {
		buildOptions = append(buildOptions, protocutil.WithExcludePattern(excludePattern))
	}
	for _, extraInclude := range options.ExtraIncludes {
		buildOptions = append(buildOptions, protocutil.WithExtraInclude(extraInclude))
	}
	return protocutil.NewBuilder(builderOptions...).Build(options.ProjectRoot, options.InputDirPath, options.OutputDirPath, buildOptions...)
}

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}
