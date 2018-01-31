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
	"github.com/pkg/errors"
)

var CustomRep tally.StatsReporter
var ErrorReported bool

func init() {
	ErrorReported = false
	toolname := "dep"
	if flag.Lookup("test.v") != nil {
		toolname = toolname + "-tests"
	}
	var err error
	CustomRep, err = reporter.New(toolname, reporter.WithSample(1.0))
	if err != nil {
		CustomRep = tally.NullStatsReporter
		UberLogger.Println("Falling back to a null stats reporter")
	}
}

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

func GetRepoTagsFromWorkingDirectory() (map[string]string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get working directory")
	}
	tags := GetRepoTagFromRoot(pwd)
	return tags, nil
}

func SetTag(tags map[string]string, key string, value string) {
	if tags != nil {
		tags[key] = value
	}
}

func Instrument(name string, tags map[string]string) func() {
	defer catchErrors()

	scope, closer := getNewScope(tags)

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

	if !ErrorReported {
		scope, closer := getNewScope(tags)

		scope.Counter(name).Inc(1)
		if err := closer.Close(); err != nil {
			UberLogger.Print(err.Error())
		}
		ErrorReported = true
	}
}

func catchErrors() {
	if r := recover(); r != nil {
		UberLogger.Printf("Got error while trying to report usage data: %s", r)
	}
}

func getNewScope(tags map[string]string) (tally.Scope, io.Closer) {
	if tags == nil {
		tags = make(map[string]string)
	}

	if os.Getenv(TurnOffUberDeduceLogicEnv) == "" {
		tags["DeducerLogic"] = "Uber"
	} else {
		tags["DeducerLogic"] = "BaseDep"
	}

	scope, closer := tally.NewRootScope(
		tally.ScopeOptions{Reporter: CustomRep, Tags: tags},
		5*time.Second,
	)

	return scope, closer
}
