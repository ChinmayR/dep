package err

import "fmt"

// NoConfig represents no configuration files being found.
type NoConfig struct {
	Dirs []string
}

func (err NoConfig) Error() string {
	return fmt.Sprintf("no configuration files found in directories %q", err.Dirs)
}
