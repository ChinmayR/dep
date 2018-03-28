package uber

import (
	"flag"
	"io"
	"path/filepath"
	"regexp"
	"time"
	"github.com/rs/xid"
	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"github.com/uber-go/tally"
	"strings"
	"os"
)

var CustomRep tally.StatsReporter
var scope tally.Scope
var scopeCloser io.Closer
var errorTags map[string]int64
var runStatus string
var runId string

//All error types that dep can generate in solve_failures.go
const (
	NO_VERSION_FOUND_ERROR                         		= "no_version_found"
	NO_VERSION_MET_CONSTRAINT_ERROR                		= "no_version_met_constraints"
	CASE_MISMATCH_ERROR                            		= "case_mismatch"
	DISJOINT_CONSTRAINT_ERROR                      		= "disjoint_constraint"
	CONSTRAINT_NOT_ALLOWED_ERROR                 		= "constraint_not_allowed"
	VERSION_NOT_ALLOWED_ERROR                    		= "version_not_allowed"
	MISSING_SOURCE_ERROR                        	 	= "missing_source"
	BAD_OPTS_ERROR                              	 	= "bad_opts"
	SOURCE_MISMATCH_ERROR                       	 	= "source_mismatch"
	CHECKEE_HAS_PROBLEM_PACKAGES_ERROR          	 	= "checkee_has_problematic_packages"
	DEP_HAS_MISSING_PACKAGES_ERROR               		= "dep_has_missing_package"
	DEP_HAS_PACKAGES_WITH_UNUSABLE_GO_CODE_ERROR 		= "dep_has_packages_with_unusable_go_code"
	NON_EXISTENT_REVISION_ERROR                  		= "non_existent_revision"
)

//All tag names used in dep's metrics
const (
	REPO_TAG		= "repo"
	COMMAND_TAG		= "command"
	RUNID_TAG		= "runid"
	STATUS_TAG		= "status"
	ERROR_TAG		= "error"
	SEMVER_TAG		= "semver"
)

//All dep's metric names
const (
	LATENCY_METRIC		= "latency"
	FAILURE_METRIC		= "failure"
	FREQUENCY_METRIC 	= "frequency"
	CC_METRIC		= "ccfreq"
	//All error metric names are the same as the error types const above
)

//The final result of running a dep command
const (
	SUCCESSFUL_RUN 	= "success"
	FAILED_RUN 	= "failure"
)

//When major version changes, queries on Grafana's dashboard should change too
const METRICS_STABLE_VERSION = "1.0.0"

func init() {
	runId = xid.New().String()
	errorTags = make(map[string]int64)
	runStatus = FAILED_RUN
	toolname := "uber_dep"
	if flag.Lookup("test.v") != nil || os.Getenv(RunningIntegrationTests) == "yes" {
		toolname = toolname + "-tests"
	}
	var err error
	CustomRep, err = reporter.New(toolname, reporter.WithSample(1.0))
	scope, scopeCloser = tally.NewRootScope(tally.ScopeOptions{Reporter: CustomRep}, 5*time.Second)
	if err != nil {
		CustomRep = tally.NullStatsReporter
		UberLogger.Println("Falling back to a null stats reporter")
	}
}

func getRepoTagFriendlyNameFromCWD(cwd string) string {
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
		defer catchErrors()
		latency := time.Since(start)
		repo := getRepoTagFriendlyNameFromCWD(repoName)
		addLatencyMetric(cmd, repo, latency, cmdFlags)
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
func ReportClearCacheMetric() {
	defer catchErrors()
	tags := getVersionedTagMap()
	scope.Tagged(tags).Counter(CC_METRIC).Inc(1)
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


//Latency metric measures that time it takes to execute a single dep command. Associated tags are:
//- status: can be either "success" or "failure" based on whether dep succeeded or failed to resolve dependencies
//- Other common tags. Refer to getCommonTags method for the rest of associated tags
func addLatencyMetric(cmd string, repo string, latency time.Duration, cmdFlags map[string]string) {
	tags := getCommonTags(repo, cmd)
	for k,v := range cmdFlags {
		tags[k] = v
	}
	tags[STATUS_TAG] = runStatus
	scope.Tagged(tags).Timer(LATENCY_METRIC).Record(latency)
}


//Failure metric is reported when dep fails to resolve dependencies for a repo with or without retries.
//Associated tags are:
//- error: the list of errors that caused the failure. The list is a string of concatenated one or more error types
//separated by a "."
//- Other common tags. Refer to getCommonTags method for the rest of associated tags
func addFailureMetric(cmd string, repo string) {
	if runStatus == FAILED_RUN {
		tags := getCommonTags(repo, cmd)
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
//Refer to getCommonTags method for the complete list of associated tags.
//* the name of each metric is an error type from the const error list
func addErrorMetrics(cmd string, repo string) {
	tags := getCommonTags(repo, cmd)
	for errorName,errorCount := range errorTags {
		if errorCount > 0 {
			scope.Tagged(tags).Counter(errorName).Inc(errorCount)
		}
	}
}


//Frequency metric is reported to calculate dep's adoption and per repo usage.
//Refer to getCommonTags method for the complete list of associated tags.
func addFrequencyMetric(repo string, cmd string) {
	tags := getCommonTags(repo, cmd)
	scope.Tagged(tags).Counter(FREQUENCY_METRIC).Inc(1)
}

//Creates a string map that contains the tag name/value pairs.
//This is the common tag list used in repo metrics reporting. The map includes the following tags:
//- runid: a unique ID for a single dep run. This ID is shared across all metrics reported per run
//- repo: the name of the repository on which dep ran
//- command: the command name
//- semver: the current stable metrics semantic version
func getCommonTags(repo string, cmd string) map[string]string {
	tags := getVersionedTagMap()
	tags[RUNID_TAG] = runId
	tags[REPO_TAG] = repo
	tags[COMMAND_TAG] = cmd
	return tags
}

func getVersionedTagMap() map[string]string {
	tags := make(map[string]string)
	tags[SEMVER_TAG] = METRICS_STABLE_VERSION
	return tags
}

func catchErrors() {
	if r := recover(); r != nil {
		UberLogger.Printf("Got error while trying to report usage data: %s", r)
	}
}
