package load

import (
	"errors"
	"os"
	"path/filepath"

	"go.uber.org/config"
)

// LookUpFunc is a type alias for a function to look for environment variables,
type LookUpFunc func(string) (string, bool)

// FileInfo represents a file to load by LoadFromFiles function.
type FileInfo struct {
	Name        string // Name of the file.
	Interpolate bool   // Interpolate contents?
}

// TestProvider will read configuration base.yaml and test.yaml from a
func TestProvider() (config.Provider, error) {
	return FromFiles(
		[]string{"config"},
		[]FileInfo{
			{Name: "base.yaml", Interpolate: true},
			{Name: "test.yaml", Interpolate: true},
		},
		os.LookupEnv)
}

// FromFiles reads configuration files from dirs using lookUp function
// for interpolation. First both slices are interpolated with the provided
// lookUp function. Then all the files are loaded from the all dirs.
// For example:
//
//   FromFiles([]string{"dir1", "dir2"},[]FileInfo{{"base.yaml"},{"test.yaml"}}, nil)
//
// will try to load files in this order:
//  1. dir1/base.yaml
//  2. dir2/base.yaml
//  3. dir1/test.yaml
//  4. dir2/test.yaml
// The function will return an error, if there are no providers to load.
func FromFiles(dirs []string, files []FileInfo, lookUp LookUpFunc) (config.Provider, error) {
	var providers []config.Provider
	for _, info := range files {
		for _, dir := range dirs {
			name := filepath.Join(dir, info.Name)

			if _, err := os.Stat(name); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, err
			}

			if info.Interpolate {
				p, err := config.NewYAMLProviderWithExpand(lookUp, name)
				if err != nil {
					return nil, err
				}

				providers = append(providers, p)
			} else {
				p, err := config.NewYAMLProviderFromFiles(name)
				if err != nil {
					return nil, err
				}

				providers = append(providers, p)
			}
		}
	}

	if len(providers) == 0 {
		return nil, errors.New("no providers were loaded")
	}

	return config.NewProviderGroup("files", providers...)
}
