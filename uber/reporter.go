package uber

import (
	"flag"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"github.com/uber-go/tally"
)

var CustomRep tally.StatsReporter
var scope tally.Scope
var scopeCloser io.Closer
var errorTags map[string]int64
var runStatus string
var latencyNormFactor int

//All error types that dep can generate in solve_failures.go
const (
	NO_VERSION_FOUND_ERROR                       = "no_version_found"
	NO_VERSION_MET_CONSTRAINT_ERROR              = "no_version_met_constraints"
	CASE_MISMATCH_ERROR                          = "case_mismatch"
	DISJOINT_CONSTRAINT_ERROR                    = "disjoint_constraint"
	CONSTRAINT_NOT_ALLOWED_ERROR                 = "constraint_not_allowed"
	VERSION_NOT_ALLOWED_ERROR                    = "version_not_allowed"
	MISSING_SOURCE_ERROR                         = "missing_source"
	BAD_OPTS_ERROR                               = "bad_opts"
	SOURCE_MISMATCH_ERROR                        = "source_mismatch"
	CHECKEE_HAS_PROBLEM_PACKAGES_ERROR           = "checkee_has_problematic_packages"
	DEP_HAS_MISSING_PACKAGES_ERROR               = "dep_has_missing_package"
	DEP_HAS_PACKAGES_WITH_UNUSABLE_GO_CODE_ERROR = "dep_has_packages_with_unusable_go_code"
	NON_EXISTENT_REVISION_ERROR                  = "non_existent_revision"
)

//All tag names used in dep's metrics
const (
	REPO_TAG        = "repo"
	COMMAND_TAG     = "command"
	USER_TAG        = "user"
	RUNID_TAG       = "runid"
	STATUS_TAG      = "status"
	ERROR_TAG       = "error"
	SEMVER_TAG      = "semver"
	DEP_VERSION_TAG = "depversion"
)

//All dep's metric names
const (
	LATENCY_METRIC       = "latency"
	NORM_LATENCY_METRIC  = "normlatency"
	FAILURE_METRIC       = "failure"
	FREQUENCY_METRIC     = "frequency"
	CC_METRIC            = "ccfreq"
	INT_SIG_METRIC       = "intsig"
	VQS_EXHAUSTED_METRIC = "vqsexhausted"
	PANIC_METRIC         = "panic"
	//All error metric names are the same as the error types const above
)

//The final result of running a dep command
const (
	SUCCESSFUL_RUN = "success"
	FAILED_RUN     = "failure"
)

//When major version changes, queries on Grafana's dashboard should change too
const METRICS_STABLE_VERSION = "1.0.1"

func init() {
	errorTags = make(map[string]int64)
	runStatus = FAILED_RUN
	toolname := "uber_dep"
	if flag.Lookup("test.v") != nil || os.Getenv(RunningIntegrationTests) == "yes" {
		toolname = toolname + "-tests"
	}
	var err error
	CustomRep, err = reporter.New(toolname, reporter.WithSample(1.0))
	if err != nil {
		CustomRep = tally.NullStatsReporter
	}
	scope, scopeCloser = tally.NewRootScope(tally.ScopeOptions{Reporter: CustomRep}, 5*time.Second)
}

func GetRepoTagFriendlyNameFromCWD(cwd string) string {
	repo := filepath.Base(cwd)
	// Forbidden characters for tag values in M3, see
	// https://engdocs.uberinternal.com/m3_and_umonitor/intro/data_model.html#invalid-characters
	r := regexp.MustCompile(`[\+,=\s\:\|]`)
	repo = r.ReplaceAllString(repo, "-")
	return strings.TrimSuffix(repo, ".git")
}

//dep reports four types of metrics:
//1. Latency - timer metric
//2. Failure - counter metric
//3. Frequency - counter metric
//4. Error(s) - counter metric
func ReportRepoMetrics(cmd string, repoName string, cmdFlags map[string]string) func() {
	defer catchErrors()
	start := time.Now()
	return func() {
		if os.Getenv(TurnOffMetricsReporting) != "" {
			DebugLogger.Println("Metrics reporting is turned off, so skipping reporting any metrics for this run.")
			return
		}
		defer catchErrors()
		latency := time.Since(start)
		repo := GetRepoTagFriendlyNameFromCWD(repoName)
		addLatencyMetric(cmd, repo, latency, cmdFlags)
		addNormLatencyMetric(cmd, repo, latency, cmdFlags)
		addFailureMetric(cmd, repo)
		addFrequencyMetric(repo, cmd)
		addErrorMetrics(cmd, repo)
		if err := scopeCloser.Close(); err != nil {
			UberLogger.Print(err.Error())
		}
	}
}

//dep reports clear cache counts via this metric.
//Refer to getVersionedTagMap method more info about the semantic versioning associated tag
func ReportClearCacheMetric(cmd string) {
	defer catchErrors()
	tags := getCommonTagsWithCmd(cmd)
	scope.Tagged(tags).Counter(CC_METRIC).Inc(1)
	if err := scopeCloser.Close(); err != nil {
		UberLogger.Print(err.Error())
	}
}

//report this counter metric when an interrupt signal is received
func ReportInterruptSignalReceivedMetric(repo string, cmdName string) {
	defer catchErrors()
	tags := getCommonTagsWithCmdAndRepo(repo, cmdName)
	scope.Tagged(tags).Counter(INT_SIG_METRIC).Inc(1)
	if err := scopeCloser.Close(); err != nil {
		UberLogger.Print(err.Error())
	}
}

//report this counter metric when we fail solver from too many
//version queue exhaustions
func ReportVQSExhaustedLimitReachedMetric(repo string) {
	defer catchErrors()
	tags := getCommonTags()
	tags[REPO_TAG] = repo
	scope.Tagged(tags).Counter(VQS_EXHAUSTED_METRIC).Inc(1)
	if err := scopeCloser.Close(); err != nil {
		UberLogger.Print(err.Error())
	}
}

//report this counter metric when a panic is recovered
func ReportPanicMetric() {
	defer catchErrors()
	tags := getCommonTags()
	scope.Tagged(tags).Counter(PANIC_METRIC).Inc(1)
	if err := scopeCloser.Close(); err != nil {
		UberLogger.Print(err.Error())
	}
}

//Called to report an error from the const error list
func ReportError(errorName string) {
	errorTags[errorName]++
}

//Only called when a dep command succeeds with or without errors
func ReportSuccess() {
	runStatus = SUCCESSFUL_RUN
}

//Only called when there is a factor to divide the latency by
func LatencyNormFactor(factor int) {
	latencyNormFactor = factor
}

//Latency metric measures that time it takes to execute a single dep command. Associated tags are:
//- status: can be either "success" or "failure" based on whether dep succeeded or failed to resolve dependencies
//- Other common tags. Refer to getCommonTagsWithCmd method for the rest of associated tags
func addLatencyMetric(cmd string, repo string, latency time.Duration, cmdFlags map[string]string) {
	tags := getCommonTagsWithCmdAndRepo(repo, cmd)
	for k, v := range cmdFlags {
		tags[k] = v
	}
	tags[STATUS_TAG] = runStatus
	scope.Tagged(tags).Timer(LATENCY_METRIC).Record(latency)
}

//Norm Latency metric measures that time it takes to execute a single dep command divided by the number of
//packages it dealt with. Associated tags are:
//- status: can be either "success" or "failure" based on whether dep succeeded or failed to resolve dependencies
//- Other common tags. Refer to getCommonTagsWithCmd method for the rest of associated tags
func addNormLatencyMetric(cmd string, repo string, latency time.Duration, cmdFlags map[string]string) {
	tags := getCommonTagsWithCmdAndRepo(repo, cmd)
	for k, v := range cmdFlags {
		tags[k] = v
	}
	tags[STATUS_TAG] = runStatus

	if latencyNormFactor == 0 {
		latencyNormFactor = 1
	}
	scope.Tagged(tags).Timer(NORM_LATENCY_METRIC).Record(latency / time.Duration(latencyNormFactor))
}

//Failure metric is reported when dep fails to resolve dependencies for a repo with or without retries.
//Associated tags are:
//- error: the list of errors that caused the failure. The list is a string of concatenated one or more error types
//separated by a "."
//- Other common tags. Refer to getCommonTagsWithCmd method for the rest of associated tags
func addFailureMetric(cmd string, repo string) {
	if runStatus == FAILED_RUN {
		tags := getCommonTagsWithCmdAndRepo(repo, cmd)
		var errorElements []string
		for k := range errorTags {
			if errorTags[k] > 0 {
				errorElements = append(errorElements, k)
			}
		}
		tags[ERROR_TAG] = strings.Join(errorElements, ".")
		scope.Tagged(tags).Counter(FAILURE_METRIC).Inc(1)
	}
}

//*Error metrics are all the error types that occurred during a single dep run. Each encountered error is reported
//as a separate metric. That helps calculate the error count per error type in Grafana.
//Refer to getCommonTagsWithCmd method for the complete list of associated tags.
//* the name of each metric is an error type from the const error list
func addErrorMetrics(cmd string, repo string) {
	tags := getCommonTagsWithCmdAndRepo(repo, cmd)
	for errorName, errorCount := range errorTags {
		if errorCount > 0 {
			scope.Tagged(tags).Counter(errorName).Inc(errorCount)
		}
	}
}

//Frequency metric is reported to calculate dep's adoption and per repo usage.
//Refer to getCommonTagsWithCmd method for the complete list of associated tags.
func addFrequencyMetric(repo string, cmd string) {
	tags := getCommonTagsWithCmdAndRepo(repo, cmd)
	scope.Tagged(tags).Counter(FREQUENCY_METRIC).Inc(1)
}

//Creates a string map that contains the tag name/value pairs.
//This is the common tag list used in repo metrics reporting.
//This map includes the following tags along with the common tags:
//- repo: the name of the repository on which dep ran
//- command: the command name
func getCommonTagsWithCmdAndRepo(repo string, cmd string) map[string]string {
	tags := getCommonTags()
	tags[COMMAND_TAG] = cmd
	tags[REPO_TAG] = repo
	return tags
}

//Creates a string map that contains the tag name/value pairs.
//This is the common tag list used in repo metrics reporting.
//This map includes the following tags along with the common tags:
//- command: the command name
func getCommonTagsWithCmd(cmd string) map[string]string {
	tags := getCommonTags()
	tags[COMMAND_TAG] = cmd
	return tags
}

//Creates a string map that contains the tag name/value pairs.
//This is the common tag list used in repo metrics reporting. The map includes the following tags:
//- user: the username from the golang user package
//- runid: a unique ID for a single dep run. This ID is shared across all metrics reported per run
//- semver: the current stable metrics semantic version
//- depversion: the current stable version of dep
func getCommonTags() map[string]string {
	tags := make(map[string]string)
	curUser, err := user.Current()
	if err == nil {
		tags[USER_TAG] = curUser.Username
	}
	tags[RUNID_TAG] = RunId
	tags[SEMVER_TAG] = METRICS_STABLE_VERSION
	tags[DEP_VERSION_TAG] = DEP_VERSION
	return tags
}

func catchErrors() {
	if r := recover(); r != nil {
		UberLogger.Printf("Got error while trying to report usage data: %s", r)
	}
}
