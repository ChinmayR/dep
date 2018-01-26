package uber

import "os"

func SetEnvVar(envVar string, val string) func() {
	old := os.Getenv(envVar)
	os.Setenv(envVar, val)

	return func() {
		os.Setenv(envVar, old)
	}
}

func SetAndUnsetEnvVar(envVar string, val string) func() {
	os.Setenv(envVar, val)
	return func () {
		os.Unsetenv(envVar)
	}
}
