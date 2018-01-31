// Package protocutil provides helper functionality to use the protoc tool.
package protocutil

import "os/exec"

const (
	// DefaultGoPlugin is the default golang protoc plugin.
	DefaultGoPlugin = "gogoslick"
	// DefaultAddGRPC is the default value for whether to compile gRPC as well.
	DefaultAddGRPC = true
	// DefaultProtocPath is the default path to use for protoc.
	DefaultProtocPath = "protoc"
)

// BuildOption is an option for a Build call.
type BuildOption func(*buildOptions)

// WithExcludePattern returns a new BuildOption specifying to exclude
// proto files that match the given pattern.
func WithExcludePattern(excludePattern string) BuildOption {
	return func(buildOptions *buildOptions) {
		buildOptions.ExcludePatterns = append(buildOptions.ExcludePatterns, excludePattern)
	}
}

// WithExtraInclude returns a new BuildOption specifying to include
// the given directory with -I. By default, the input directory
// is already included.
func WithExtraInclude(extraInclude string) BuildOption {
	return func(buildOptions *buildOptions) {
		buildOptions.ExtraIncludes = append(buildOptions.ExtraIncludes, extraInclude)
	}
}

// Builder builds proto files.
type Builder interface {
	Build(projectRoot string, inputDirPath string, outputDirPath string, options ...BuildOption) error
}

// BuilderOption is an option for a new Builder.
type BuilderOption func(*builder)

// WithGoPlugin is an option to set the golang protoc plugin.
func WithGoPlugin(goPlugin string) BuilderOption {
	return func(builder *builder) {
		builder.GoPlugin = goPlugin
	}
}

// WithAddGRPC is an option to set whether to compile gRPC.
func WithAddGRPC(addGRPC bool) BuilderOption {
	return func(builder *builder) {
		builder.AddGRPC = addGRPC
	}
}

// WithExtraGoPlugin is an option to add an extra golang plugin.
func WithExtraGoPlugin(extraGoPlugin string) BuilderOption {
	return func(builder *builder) {
		builder.ExtraGoPlugins = append(builder.ExtraGoPlugins, extraGoPlugin)
	}
}

// WithProtocPath is an option to specify the protoc path to use.
func WithProtocPath(protocPath string) BuilderOption {
	return func(builder *builder) {
		builder.ProtocPath = protocPath
	}
}

// WithLogFunc returns a new BuilderOption specifying the Builder to use
// the given logging function. By default, no logs are produced.
func WithLogFunc(logFunc func(string, ...interface{})) BuilderOption {
	return func(builder *builder) {
		builder.LogFunc = logFunc
	}
}

// NewBuilder creates a new Builder.
func NewBuilder(options ...BuilderOption) Builder {
	return newBuilder(options...)
}

type buildOptions struct {
	ExcludePatterns []string
	ExtraIncludes   []string
}

func newBuildOptions(options ...BuildOption) *buildOptions {
	buildOptions := &buildOptions{}
	for _, option := range options {
		option(buildOptions)
	}
	return buildOptions
}

// withCmdRunner uses the given Runner for running protoc commands.
//
// This is used for testing.
func withCmdRunner(cmdRunner func(*exec.Cmd) error) BuilderOption {
	return func(builder *builder) {
		builder.CmdRunner = cmdRunner
	}
}
