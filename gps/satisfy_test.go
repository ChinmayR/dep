package gps

import (
	"testing"
)

func Test_IsSirupsenLogrusCaseMismatch(t *testing.T) {
	type caseMismatchTestCase struct {
		pr         ProjectRoot
		pr2        ProjectRoot
		isSirupsen bool
	}

	cases := map[string]caseMismatchTestCase{
		"sirupsenCase": {
			pr:         "github.com/sirupsen/logrus",
			pr2:        "github.com/Sirupsen/logrus",
			isSirupsen: true,
		},
		"sirupsenCaseFlipped": {
			pr:         "github.com/Sirupsen/logrus",
			pr2:        "github.com/sirupsen/logrus",
			isSirupsen: true,
		},
		"notSirupsenCase": {
			pr:         "github.com/Sirupsen/logrus",
			pr2:        "github.com/golang/dep",
			isSirupsen: false,
		},
	}

	for tcName, tc := range cases {
		want := tc.isSirupsen
		got := isSirupsenLogrusCaseMismatch(tc.pr, tc.pr2)

		if got != want {
			t.Fatalf("%v: Expected %v but got %v", tcName, want, got)
		}
	}
}
