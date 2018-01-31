package configfx

import (
	"os"

	"code.uber.internal/go/configfx.git/load"
	"code.uber.internal/go/envfx.git"
	"go.uber.org/config"
)

type metaConfiguration struct {
	Fileset []string `yaml:"files"`
}

func metaCfg(context envfx.Context) (*metaConfiguration, error) {
	metaFiles := []load.FileInfo{{Name: "meta.yaml", Interpolate: true}}
	metaCfgP, err := load.FromFiles(context.ConfigDirs(), metaFiles, os.LookupEnv)
	if err != nil && !IsNoFilesFoundErr(err) {
		return nil, err
	}
	meta := &metaConfiguration{}
	if metaCfgP == nil {
		return meta, nil
	}
	err = metaCfgP.Get(config.Root).Populate(meta)
	meta.Fileset = dedupeStringSlice(meta.Fileset)
	return meta, err
}

// dedupeStringSlice removes duplicates, preserving priority (so last occurrence of a dupe is the one that will stay)
func dedupeStringSlice(slice []string) []string {
	counts := make(map[string]int)
	for _, s := range slice {
		counts[s] = counts[s] + 1
	}

	out := make([]string, 0, len(slice))
	for _, s := range slice {
		counts[s] = counts[s] - 1
		if counts[s] == 0 {
			out = append(out, s)
		}
	}
	return out
}
