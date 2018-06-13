package uber

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/dep/uber/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_IsLatestVersion(t *testing.T) {

	type testcase struct {
		inputVersion   string
		lsRemoteOutput string
		isLatest       bool
		latestVersion  string
		expectedError  bool
		errorString    string
	}

	cases := map[string]testcase{
		"equal versions": {
			inputVersion: "v0.1.0-UBER",
			lsRemoteOutput: `
05c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.1.0-UBER
						`,
			isLatest:      true,
			latestVersion: "0.1.0-UBER",
			expectedError: false,
		},
		"later versions available": {
			inputVersion: "v0.1.0-UBER",
			lsRemoteOutput: `
05c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.1.0-UBER
15c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.2.0-UBER
25c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.10.0-UBER
						`,
			isLatest:      false,
			latestVersion: "0.10.0-UBER",
			expectedError: false,
		},
		"current version is later than latest": {
			inputVersion: "v0.3.0-UBER",
			lsRemoteOutput: `
05c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.1.0-UBER
15c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.2.0-UBER
				`,
			isLatest:      true,
			latestVersion: "0.3.0-UBER",
			expectedError: true,
			errorString:   "current version 0.3.0-UBER later than latest 0.2.0-UBER",
		},
		"no data from ls remote error": {
			inputVersion:   "v0.3.0-UBER",
			lsRemoteOutput: ``,
			isLatest:       false,
			latestVersion:  "",
			expectedError:  true,
			errorString:    "no data returned from ls-remote",
		},
		"no latest version found error": {
			inputVersion: "v0.3.0-UBER",
			lsRemoteOutput: `
05c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/heads/head2
15c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/heads/head1`,
			isLatest:      false,
			latestVersion: "",
			expectedError: true,
			errorString:   "no latest version found",
		},
		"combination of semver and arbritary tags": {
			inputVersion: "v0.2.0-UBER",
			lsRemoteOutput: `
05c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.1.0-UBER
25c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/head2
15c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/v0.3.0-UBER
35c40eba7fa5512c3a161e4e9df6c8fefde75158	refs/tags/head1`,
			isLatest:      false,
			latestVersion: "0.3.0-UBER",
			expectedError: false,
		},
	}

	for tcName, tc := range cases {
		ex := &mocks.ExecutorInterface{}
		CmdExecutor = ex

		ex.On("ExecCommand", "git", time.Duration(30*time.Second), false, mock.AnythingOfType("[]string"),
			"ls-remote", "--tags", REMOTE_URL).Return(tc.lsRemoteOutput, "", nil)

		versionInfo, err := IsLatestVersion(tc.inputVersion)
		if tc.expectedError {
			if err == nil {
				t.Fatalf("%v: Expected err but got none", tcName)
			} else if !strings.EqualFold(tc.errorString, err.Error()) {
				t.Fatalf("%v: Expected error %v but got %v", tcName, tc.errorString, err.Error())
			}
		} else {
			assert.Nil(t, err)
		}

		if versionInfo.IsLatest != tc.isLatest {
			t.Fatalf("%v: Expected isLatest %v but got %v", tcName, tc.isLatest, versionInfo.IsLatest)
		}
		if versionInfo.LatestVersion != tc.latestVersion {
			t.Fatalf("%v: Expected latestVersion %v but got %v", tcName, tc.latestVersion, versionInfo.LatestVersion)
		}
		ex.AssertExpectations(t)
	}
}

func Test_IsCurrentVersionLaterThanLockVersion(t *testing.T) {

	type testcase struct {
		depVersion  string
		lockVersion string
		errExpected error
	}

	cases := map[string]testcase{
		"depVersion is older than locked version": {
			depVersion:  "v0.11.0-UBER",
			lockVersion: "v0.12.0-UBER",
			errExpected: ErrDepVersionOlder,
		},
		"depVersion is newer than locked version": {
			depVersion:  "v0.12.0-UBER",
			lockVersion: "v0.11.0-UBER",
			errExpected: nil,
		},
		"invalid semver version": {
			depVersion:  "invalidSemver",
			lockVersion: "v0.11.0-UBER",
			errExpected: ErrParsingSemVer,
		},
	}

	for tcName, tc := range cases {
		err := IsCurrentVersionLaterThanLockVersion(tc.lockVersion, tc.depVersion)
		if tc.errExpected != nil {
			if !reflect.DeepEqual(tc.errExpected, err) {
				t.Fatalf("%v: Expected error %v but got %v", tcName, tc.errExpected, err.Error())
			}
		} else {
			assert.Nil(t, err)
		}
	}
}
