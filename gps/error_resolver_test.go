package gps

import (
	"io/ioutil"
	"log"
	"reflect"
	"testing"
)

func TestHandleError(t *testing.T) {
	testCases := map[string]struct {
		err      error
		expected []OverridePackage
		wantErr  bool
	}{
		"no version matches requirement": {
			err: &noVersionError{
				pn: mkPI("foo"),
				fails: []failedVersion{
					{
						v: NewVersion("2.1.3"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("foo 2.1.3"),
							failparent: []dependency{mkDep("root", "foo ^1.0.0", "foo")},
							c:          mkSVC("^1.0.0"),
						},
					},
					{
						v: NewVersion("2.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("foo 2.0.0"),
							failparent: []dependency{mkDep("root", "foo ^1.0.0", "foo")},
							c:          mkSVC("^1.0.0"),
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "foo",
					Constraint: "^1.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "foo",
					Constraint: "2.1.3",
					Source:     "",
				},
				OverridePackage{
					Name:       "foo",
					Constraint: "2.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"disjoint constraint failure": {
			err: &disjointConstraintFailure{
				goal:      mkDep("foo 1.0.0", "shared <=2.0.0", "shared"),
				failsib:   []dependency{mkDep("bar 1.0.0", "shared >3.0.0", "shared")},
				nofailsib: nil,
				c:         mkSVC(">3.0.0"),
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "shared",
					Constraint: "<=2.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: ">3.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"source mismatch failure": {
			err: &sourceMismatchFailure{
				shared:   ProjectRoot("baz"),
				current:  "baz",
				mismatch: "quux",
				prob:     mkAtom("bar 2.0.0"),
				sel:      []dependency{mkDep("foo 1.0.0", "bar 2.0.0", "bar")},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "baz",
					Constraint: "",
					Source:     "baz",
				},
				OverridePackage{
					Name:       "baz",
					Constraint: "",
					Source:     "quux",
				},
			},
			wantErr: false,
		},
		"version not allowed failure": {
			err: &versionNotAllowedFailure{
				goal:       mkAtom("baz 2.0.0"),
				failparent: []dependency{mkDep("root", "baz 1.0.0", "baz/qux")},
				c:          NewVersion("1.0.0"),
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "baz",
					Constraint: "1.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "baz",
					Constraint: "2.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"no version matching combined constraint": {
			err: &noVersionError{
				pn: mkPI("shared"),
				fails: []failedVersion{
					{
						v: NewVersion("3.5.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("shared 3.5.0"),
							failparent: []dependency{mkDep("foo 1.0.0", "shared >=2.0.0, <3.0.0", "shared")},
							c:          mkSVC(">=2.9.0, <3.0.0"),
						},
					},
					{
						v: NewVersion("2.5.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("shared 2.5.0"),
							failparent: []dependency{mkDep("bar 1.0.0", "shared >=2.9.0, <4.0.0", "shared")},
							c:          mkSVC(">=2.9.0, <3.0.0"),
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "shared",
					Constraint: "^2.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: "3.5.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: ">=2.9.0, <4.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: "2.5.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"disjoint constraint": {
			err: &noVersionError{
				pn: mkPI("foo"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &disjointConstraintFailure{
							goal:      mkDep("foo 1.0.0", "shared <=2.0.0", "shared"),
							failsib:   []dependency{mkDep("bar 1.0.0", "shared >3.0.0", "shared")},
							nofailsib: nil,
							c:         mkSVC(">3.0.0"),
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "shared",
					Constraint: "<=2.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: ">3.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"no valid solution": {
			err: &noVersionError{
				pn: mkPI("foo"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &disjointConstraintFailure{
							goal:      mkDep("foo 1.0.0", "shared <=2.0.0", "shared"),
							failsib:   []dependency{mkDep("bar 1.0.0", "shared >3.0.0", "shared")},
							nofailsib: nil,
							c:         mkSVC(">3.0.0"),
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "shared",
					Constraint: "<=2.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "shared",
					Constraint: ">3.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"no version matches while backtracking": {
			err: &noVersionError{
				pn: mkPI("b"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("b 1.0.0"),
							failparent: []dependency{mkDep("root", "b >1.0.0", "b")},
							c:          mkSVC(">1.0.0"),
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "b",
					Constraint: ">1.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "b",
					Constraint: "1.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
		"basic source override failure": {
			err: &noVersionError{
				pn: mkPI("bar"),
				fails: []failedVersion{
					{
						v: NewVersion("2.0.0"),
						f: &sourceMismatchFailure{
							shared:   ProjectRoot("baz"),
							current:  "baz",
							mismatch: "quux",
							prob:     mkAtom("bar 2.0.0"),
							sel:      []dependency{mkDep("foo 1.0.0", "bar 2.0.0", "bar")},
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "baz",
					Constraint: "",
					Source:     "baz",
				},
				OverridePackage{
					Name:       "baz",
					Constraint: "",
					Source:     "quux",
				},
			},
			wantErr: false,
		},
		"heterogeneous errors": {
			err: &noVersionError{
				pn: mkPI("baz"),
				fails: []failedVersion{
					{
						v: NewVersion("2.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("baz 2.0.0"),
							failparent: []dependency{mkDep("root", "baz 1.0.0", "baz/qux")},
							c:          NewVersion("1.0.0"),
						},
					},
					{
						v: NewVersion("1.0.0"),
						f: &checkeeHasProblemPackagesFailure{
							goal: mkAtom("baz 1.0.0"),
							failpkg: map[string]errDeppers{
								"baz/qux": {
									err: nil, // nil indicates package is missing
									deppers: []atom{
										mkAtom("root"),
									},
								},
							},
						},
					},
				},
			},
			expected: []OverridePackage{
				OverridePackage{
					Name:       "baz",
					Constraint: "1.0.0",
					Source:     "",
				},
				OverridePackage{
					Name:       "baz",
					Constraint: "2.0.0",
					Source:     "",
				},
			},
			wantErr: false,
		},
	}
	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			ovrPkgs, err := HandleErrors(log.New(ioutil.Discard, "", 0), testCase.err)

			if testCase.wantErr && err == nil {
				t.Fatalf("wanted error but got none")
			}
			if !reflect.DeepEqual(testCase.expected, ovrPkgs) {
				t.Fatal("expected packages did not match")
			}
		})
	}
}
