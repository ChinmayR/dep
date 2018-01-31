package testhelper

import (
	"io/ioutil"
	"os"
)

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

// SetEnvVar sets an environment variable and returns a function
// which restores the environment to its original state.
func SetEnvVar(key, value string) (restore func()) {
	if c, ok := os.LookupEnv(key); ok {
		restore = func() {
			panicOnError(os.Setenv(key, c))
		}
	} else {
		restore = func() {
			panicOnError(os.Unsetenv(key))
		}
	}

	panicOnError(os.Setenv(key, value))
	return
}

// UnsetEnvVar unsets an environment variable and returns a function
// which restores the environment to its original state.
func UnsetEnvVar(key string) (restore func()) {
	if c, ok := os.LookupEnv(key); ok {
		restore = func() {
			panicOnError(os.Setenv(key, c))
		}
		panicOnError(os.Unsetenv(key))
	} else {
		restore = func() {}
	}
	return
}

// IsProductionEnvironment indicates if running in production environment.
var IsProductionEnvironment = isProductionEnvironment()

func isProductionEnvironment() bool {
	if environment, err := ioutil.ReadFile("/etc/uber/environment"); err == nil {
		return string(environment) == "production"
	}
	return false
}
