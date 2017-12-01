package uber

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"github.com/uber-go/tally"
)

func GetRepoTagFromRoot(root string) map[string]string {
	repo := filepath.Base(root)
	tags := make(map[string]string)
	tags["repo"] = repo
	return tags
}

func Instrument(name string, tags map[string]string) func() {
	defer catchErrors()
	toolname := "dep"
	if flag.Lookup("test.v") != nil {
		toolname = toolname + "-tests"
	}

	rep, err := reporter.New(toolname, reporter.WithSample(1.0))
	if err != nil {
		rep = tally.NullStatsReporter
	}

	if tags == nil {
		tags = make(map[string]string)
	}

	if os.Getenv(TurnOffUberDeduceLogicEnv) == "" {
		tags["DeducerLogic"] = "Uber"
	} else {
		tags["DeducerLogic"] = "BaseDep"
	}

	scope, closer := tally.NewRootScope(
		tally.ScopeOptions{Reporter: rep, Tags: tags},
		5*time.Second,
	)

	t := scope.Timer(name).Start()

	return func() {
		defer catchErrors()
		t.Stop()
		if err := closer.Close(); err != nil {
			UberLogger.Print(err.Error())
		}
	}
}

func catchErrors() {
	if r := recover(); r != nil {
		UberLogger.Printf("Got error while trying to report usage data: %s", r)
	}
}
