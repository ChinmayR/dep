// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"testing"

	"github.com/pkg/errors"
	"reflect"
	"github.com/golang/dep/uber"
)

// just need a listVersions method
type fakeBridge struct {
	*bridge
	vl []Version
}

var fakevl = []Version{
	NewVersion("v2.0.0").Pair("200rev"),
	NewVersion("v1.1.1").Pair("111rev"),
	NewVersion("v1.1.0").Pair("110rev"),
	NewVersion("v1.0.0").Pair("100rev"),
	NewBranch("master").Pair("masterrev"),
}

var allFakeFilterVl = []Version{
	NewVersion("v2.0.0").Pair("200rev"),
	NewVersion("v1.1.1").Pair("111rev"),
	NewVersion("v2.3").Pair("23rev"),
	NewBranch("non-default-branch-version-2").Pair("non-default-branch-version-2rev"),
	NewBranch("non-default-branch-version-3").Pair("non-default-branch-version-3rev"),
	NewBranch("non-default-branch-version-1").Pair("non-default-branch-version-1rev"),
	newDefaultBranch("master").Pair("masterrev"),
	newDefaultBranch("non-master-default-branch-version").Pair("non-master-default-branch-versionrev"),
}

var withNonDefaultBranchConstVl = []Version{
	NewVersion("v2.0.0").Pair("200rev"),
	NewVersion("v1.1.1").Pair("111rev"),
	NewVersion("v2.3").Pair("23rev"),
	NewBranch("non-default-branch-version-1").Pair("non-default-branch-version-1rev"),
	newDefaultBranch("master").Pair("masterrev"),
	newDefaultBranch("non-master-default-branch-version").Pair("non-master-default-branch-versionrev"),
}

var withoutNonDefaultBranchConstVl = []Version{
	NewVersion("v2.0.0").Pair("200rev"),
	NewVersion("v1.1.1").Pair("111rev"),
	NewVersion("v2.3").Pair("23rev"),
	newDefaultBranch("master").Pair("masterrev"),
	newDefaultBranch("non-master-default-branch-version").Pair("non-master-default-branch-versionrev"),
}

func init() {
	SortForUpgrade(fakevl)
}

func (fb *fakeBridge) listVersions(id ProjectIdentifier) ([]Version, error) {
	// it's a fixture, we only ever do the one, regardless of id
	return fb.vl, nil
}

type fakeFailBridge struct {
	*bridge
}

var errVQ = errors.New("vqerr")

func (fb *fakeFailBridge) listVersions(id ProjectIdentifier) ([]Version, error) {
	return nil, errVQ
}

func TestVersionQueueSetup(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(uber.UseNonDefaultVersionBranches, "yes")()
	id := ProjectIdentifier{ProjectRoot: ProjectRoot("foo")}.normalize()

	// shouldn't even need to embed a real bridge
	fb := &fakeBridge{vl: fakevl}
	ffb := &fakeFailBridge{}

	_, err := newVersionQueue(id, nil, nil, ffb, nil)
	if err == nil {
		t.Error("Expected err when providing no prefv or lockv, and injected bridge returns err from ListVersions()")
	}

	vq, err := newVersionQueue(id, nil, nil, fb, nil)
	if err != nil {
		t.Errorf("Unexpected err on vq create: %s", err)
	} else {
		if len(vq.pi) != 5 {
			t.Errorf("Should have five versions from listVersions() when providing no prefv or lockv; got %v:\n\t%s", len(vq.pi), vq.String())
		}
		if !vq.allLoaded {
			t.Errorf("allLoaded flag should be set, but wasn't")
		}

		if vq.prefv != nil || vq.lockv != nil {
			t.Error("lockv and prefv should be nil")
		}
		if vq.current() != fakevl[0] {
			t.Errorf("current should be head of fakevl (%s), got %s", fakevl[0], vq.current())
		}
	}

	lockv := fakevl[0]
	prefv := fakevl[1]
	vq, err = newVersionQueue(id, lockv, nil, fb, nil)
	if err != nil {
		t.Errorf("Unexpected err on vq create: %s", err)
	} else {
		if len(vq.pi) != 1 {
			t.Errorf("Should have one version when providing only a lockv; got %v:\n\t%s", len(vq.pi), vq.String())
		}
		if vq.allLoaded {
			t.Errorf("allLoaded flag should not be set")
		}
		if vq.lockv != lockv {
			t.Errorf("lockv should be %s, was %s", lockv, vq.lockv)
		}
		if vq.current() != lockv {
			t.Errorf("current should be lockv (%s), got %s", lockv, vq.current())
		}
	}

	vq, err = newVersionQueue(id, nil, prefv, fb, nil)
	if err != nil {
		t.Errorf("Unexpected err on vq create: %s", err)
	} else {
		if len(vq.pi) != 1 {
			t.Errorf("Should have one version when providing only a prefv; got %v:\n\t%s", len(vq.pi), vq.String())
		}
		if vq.allLoaded {
			t.Errorf("allLoaded flag should not be set")
		}
		if vq.prefv != prefv {
			t.Errorf("prefv should be %s, was %s", prefv, vq.prefv)
		}
		if vq.current() != prefv {
			t.Errorf("current should be prefv (%s), got %s", prefv, vq.current())
		}
	}

	vq, err = newVersionQueue(id, lockv, prefv, fb, nil)
	if err != nil {
		t.Errorf("Unexpected err on vq create: %s", err)
	} else {
		if len(vq.pi) != 2 {
			t.Errorf("Should have two versions when providing both a prefv and lockv; got %v:\n\t%s", len(vq.pi), vq.String())
		}
		if vq.allLoaded {
			t.Errorf("allLoaded flag should not be set")
		}
		if vq.prefv != prefv {
			t.Errorf("prefv should be %s, was %s", prefv, vq.prefv)
		}
		if vq.lockv != lockv {
			t.Errorf("lockv should be %s, was %s", lockv, vq.lockv)
		}
		if vq.current() != lockv {
			t.Errorf("current should be lockv (%s), got %s", lockv, vq.current())
		}
	}
}

func TestFilterNonDefaultBranches(t *testing.T) {
	id := ProjectIdentifier{ProjectRoot: ProjectRoot("testProjectRoot")}.normalize()

	anyConst := anyConstraint{}
	assertVersionListsEquality(t, id, allFakeFilterVl, anyConst, withoutNonDefaultBranchConstVl)

	nonConst := noneConstraint{}
	assertVersionListsEquality(t, id, allFakeFilterVl, nonConst, withoutNonDefaultBranchConstVl)

	branchConst := NewBranch("non-default-branch-version-1")
	assertVersionListsEquality(t, id, allFakeFilterVl, branchConst, withNonDefaultBranchConstVl)

	defaultBranchConst := newDefaultBranch("non-master-default-branch-version")
	assertVersionListsEquality(t, id, allFakeFilterVl, defaultBranchConst, withoutNonDefaultBranchConstVl)

	plainVersionConst := plainVersion("2.3")
	assertVersionListsEquality(t, id, allFakeFilterVl, plainVersionConst, withoutNonDefaultBranchConstVl)

	semverConstraint,_ := NewSemverConstraint("=1.0.0")
	assertVersionListsEquality(t, id, allFakeFilterVl, semverConstraint, withoutNonDefaultBranchConstVl)

	revisionConst := Revision("branchRevision")
	assertVersionListsEquality(t, id, allFakeFilterVl, revisionConst, withoutNonDefaultBranchConstVl)
}

func assertVersionListsEquality(t *testing.T, id ProjectIdentifier, allVersions []Version, constraint Constraint, expected []Version) {
	actual := filterNonDefaultBranches(allVersions, constraint, id.ProjectRoot)
	if actual == nil {
		t.Error("Error - actual version list is nil. Expected=%s Actual=%s", expected, actual)
	}
	if len(actual) != len(expected) {
		t.Error("Error - atual and expected version lists has different lengths. Actual=%s Expected=%s", len(actual), len(expected))
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Error("Expected version list does not equal actual version list. Expected=%s Actual=%s", expected, actual)
	}
}

func TestVersionQueueAdvance(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(uber.UseNonDefaultVersionBranches, "yes")()
	fb := &fakeBridge{vl: fakevl}
	id := ProjectIdentifier{ProjectRoot: ProjectRoot("foo")}.normalize()

	// First with no prefv or lockv
	vq, err := newVersionQueue(id, nil, nil, fb, nil)
	if err != nil {
		t.Fatalf("Unexpected err on vq create: %s", err)
	}

	for k, v := range fakevl[1:] {
		err = vq.advance(errors.Errorf("advancment fail for %s", fakevl[k]), nil, "")
		if err != nil {
			t.Errorf("error on advancing vq from %s to %s", fakevl[k], v)
			break
		}

		if vq.current() != v {
			t.Errorf("on advance() %v, current should be %s, got %s", k, v, vq.current())
		}
	}

	if vq.isExhausted() {
		t.Error("should not be exhausted until advancing 'past' the end")
	}
	if err = vq.advance(errors.Errorf("final advance failure"), nil, ""); err != nil {
		t.Errorf("should not error on advance, even past end, but got %s", err)
	}

	if !vq.isExhausted() {
		t.Error("advanced past end, should now report exhaustion")
	}
	if vq.current() != nil {
		t.Error("advanced past end, current should return nil")
	}

	// now, do one with both a prefv and lockv
	lockv := fakevl[2]
	prefv := fakevl[0]
	vq, err = newVersionQueue(id, lockv, prefv, fb, nil)
	if err != nil {
		t.Errorf("error creating version queue: %v", err)
	}
	if vq.String() != "[v1.1.0, v2.0.0]" {
		t.Error("stringifying vq did not have expected outcome, got", vq.String())
	}
	if vq.isExhausted() {
		t.Error("can't be exhausted, we aren't even 'allLoaded' yet")
	}

	err = vq.advance(errors.Errorf("dequeue lockv"), nil, "")
	if err != nil {
		t.Error("unexpected error when advancing past lockv", err)
	} else {
		if vq.current() != prefv {
			t.Errorf("current should be prefv (%s) after first advance, got %s", prefv, vq.current())
		}
		if len(vq.pi) != 1 {
			t.Errorf("should have just prefv elem left in vq, but there are %v:\n\t%s", len(vq.pi), vq.String())
		}
	}

	err = vq.advance(errors.Errorf("dequeue prefv"), nil, "")
	if err != nil {
		t.Error("unexpected error when advancing past prefv", err)
	} else {
		if !vq.allLoaded {
			t.Error("allLoaded should now be true")
		}
		if len(vq.pi) != 3 {
			t.Errorf("should have three remaining versions after removing prefv and lockv, but there are %v:\n\t%s", len(vq.pi), vq.String())
		}
		if vq.current() != fakevl[1] {
			t.Errorf("current should be first elem of fakevl (%s) after advancing into all, got %s", fakevl[1], vq.current())
		}
	}

	// make sure the queue ordering is still right even with a double-delete
	vq.advance(nil, nil, "")
	if vq.current() != fakevl[3] {
		t.Errorf("second elem after ListVersions() should be idx 3 of fakevl (%s), got %s", fakevl[3], vq.current())
	}
	vq.advance(nil, nil, "")
	if vq.current() != fakevl[4] {
		t.Errorf("third elem after ListVersions() should be idx 4 of fakevl (%s), got %s", fakevl[4], vq.current())
	}
	vq.advance(nil, nil, "")
	if vq.current() != nil || !vq.isExhausted() {
		t.Error("should be out of versions in the queue")
	}

	// Make sure we handle things correctly when listVersions adds nothing new
	fb = &fakeBridge{vl: []Version{lockv, prefv}}
	vq, err = newVersionQueue(id, lockv, prefv, fb, nil)
	if err != nil {
		t.Errorf("error creating version queue: %v", err)
	}
	vq.advance(nil, nil, "")
	vq.advance(nil, nil, "")
	if vq.current() != nil || !vq.isExhausted() {
		t.Errorf("should have no versions left, as ListVersions() added nothing new, but still have %s", vq.String())
	}
	err = vq.advance(nil, nil, "")
	if err != nil {
		t.Errorf("should be fine to advance on empty queue, per docs, but got err %s", err)
	}

	// Also handle it well when advancing calls ListVersions() and it gets an
	// error
	vq, err = newVersionQueue(id, lockv, nil, &fakeFailBridge{}, nil)
	if err != nil {
		t.Errorf("should not err on creation when preseeded with lockv, but got err %s", err)
	}
	err = vq.advance(nil, nil, "")
	if err == nil {
		t.Error("advancing should trigger call to erroring bridge, but no err")
	}
	err = vq.advance(nil, nil, "")
	if err == nil {
		t.Error("err should be stored for reuse on any subsequent calls")
	}

}
