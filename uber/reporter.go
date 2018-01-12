package uber

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"github.com/uber-go/tally"
)

func GetRepoTagFromRoot(root string) map[string]string {
	repo := filepath.Base(root)
	// Forbidden characters for tag values in M3, see
	// https://engdocs.uberinternal.com/m3_and_umonitor/intro/data_model.html#invalid-characters
	r := regexp.MustCompile(`[\+,=\s\:\|]`)
	repo = r.ReplaceAllString(repo, "-")
	tags := make(map[string]string)
	tags["repo"] = repo
	return tags
}

func GetRepoTagFromWorkingDirectory() map[string]string {
	pwd, _ := os.Getwd()
	tags := GetRepoTagFromRoot(pwd)
	return tags
}

func Instrument(name string, tags map[string]string) func() {
	defer catchErrors()

	scope, closer := setupReporter(tags)

	t := scope.Timer(name).Start()

	return func() {
		defer catchErrors()
		t.Stop()
		if err := closer.Close(); err != nil {
			UberLogger.Print(err.Error())
		}
	}
}

func ReportError(name string, tags map[string]string) {
	defer catchErrors()

	scope, closer := setupReporter(tags)

	scope.Counter(name).Inc(1)
	if err := closer.Close(); err != nil {
		UberLogger.Print(err.Error())
	}
}

func catchErrors() {
	if r := recover(); r != nil {
		UberLogger.Printf("Got error while trying to report usage data: %s", r)
	}
}

func setupReporter(tags map[string]string) (tally.Scope, io.Closer) {
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

	return scope, closer
}
