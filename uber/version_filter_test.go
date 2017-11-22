package uber

import (
	"testing"
)

type testcase struct {
	typedVersion string
	isValid      bool
}

var typedVersions = []testcase{
	{
		typedVersion: "master",
		isValid:      true,
	},
	{
		typedVersion: "v1.0",
		isValid:      true,
	},
	{
		typedVersion: "v5.0.0",
		isValid:      true,
	},
	{
		typedVersion: "random-branch",
		isValid:      true,
	},
	{
		typedVersion: "random-remote/random-branch",
		isValid:      true,
	},
	{
		typedVersion: "phabricator/base/2192296",
		isValid:      false,
	},
	{
		typedVersion: "farc/revisions/D1118353",
		isValid:      false,
	},
}

func TestIsValidVersion(t *testing.T) {
	for _, tv := range typedVersions {
		t.Run(tv.typedVersion, func(t *testing.T) {
			t.Parallel()

			want := tv.isValid
			got := IsValidVersion(tv.typedVersion)

			if got != want {
				t.Errorf("Version filter did not match expectation for version %s:\n\t(GOT) %t\n\t(WNT) %t",
					tv.typedVersion, got, want)
			}
		})
	}
}
