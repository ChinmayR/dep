// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/internal/test"
	"github.com/golang/dep/internal/importers/base"
	"github.com/golang/dep/internal/importers/importertest"
)

func TestAppendBasicOverrides(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	testCases := map[string]struct {
		existPkgs []base.ImportedPackage
		impPkgs []base.ImportedPackage
		wantErr bool
	}{
		"overlapping override ref throws no error": {
			existPkgs: []base.ImportedPackage{
				{
					Name:      "golang.org/x/net",
					ConstraintHint: importertest.V1Constraint,
					Source:    "",
					IsOverride: true,
				},
			},
			impPkgs: []base.ImportedPackage{
				{
					Name:           "golang.org/x/net",
					LockHint:       "",
					Source:         "golang.org/x/net",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/sys",
					LockHint:       "",
					Source:         "golang.org/x/sys",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/tools",
					LockHint:       "",
					Source:         "golang.org/x/tools",
					ConstraintHint: "",
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
	}
	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			impPkgs, err := appendBasicOverrides(testCase.existPkgs)
			if testCase.wantErr && err == nil {
				t.Fatalf("wanted error but got none")
			}
			if !equalImpPkgs(impPkgs, testCase.impPkgs) {
				t.Fatal("imported packages did not match")
			}
		})
	}
}

func equalImpPkgs(a, b []base.ImportedPackage) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for _, i := range a {
		found := false
		for _, j := range b {
			if i.Name == j.Name && i.ConstraintHint == j.ConstraintHint && i.Source == j.Source &&
				i.LockHint == j.LockHint && i.IsOverride == j.IsOverride {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}