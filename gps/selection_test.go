// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"container/heap"
	"reflect"
	"testing"
)

// Regression test for https://github.com/sdboyer/gps/issues/174
func TestUnselected(t *testing.T) {
	// We don't need a comparison function for this test
	bmi1 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo", "bar"},
	}
	bmi2 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo", "bar", "baz", "baz4"},
	}
	bmi3 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo"},
	}
	bmi4 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo", "foo2", "foo3", "foo4"},
	}

	u := &unselected{
		sl: []bimodalIdentifier{bmi1, bmi2, bmi3, bmi2, bmi4},
	}
	u.cmp = u.compa

	u.remove(bimodalIdentifier{
		id: mkPI("other"),
		pl: []string{"other"},
	})

	if len(u.sl) != 5 {
		t.Fatalf("len of unselected slice should have been 5 after no-op removal, got %v", len(u.sl))
	}

	u.remove(bmi3)
	want := []bimodalIdentifier{bmi4, bmi2, bmi1, bmi2}
	if len(u.sl) != 4 {
		t.Fatalf("removal of matching bmi did not work, slice should have 4 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
	}

	heap.Push(u, bmi4)
	want = []bimodalIdentifier{bmi4, bmi2, bmi1, bmi2}
	if len(u.sl) != 4 {
		t.Fatalf("adding an existing bmi should not work, slice should have 4 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
	}

	u.remove(bmi3)
	if len(u.sl) != 4 {
		t.Fatalf("removal of bmi w/non-matching packages should be a no-op but wasn't; slice should have 4 items but has %v", len(u.sl))
	}

	u.remove(bmi2)
	want = []bimodalIdentifier{bmi4, bmi1}
	if len(u.sl) != 2 {
		t.Fatalf("removal of matching bmi did not remove all occurances, slice should have 2 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
	}
}

func TestRemoval(t *testing.T) {
	// We don't need a comparison function for this test
	bmi1 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo1", "foo2", "foo3", "foo4"},
	}
	bmi2 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"bar1", "bar2", "bar3", "bar4"},
	}

	u := &unselected{
		sl: []bimodalIdentifier{bmi1, bmi2, bmi2, bmi1, bmi2},
	}
	u.cmp = u.compa

	u.remove(bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"bar1", "bar2", "bar3", "bar5"},
	})
	want := []bimodalIdentifier{bmi1, bmi1}
	if len(u.sl) != 5 {
		t.Fatalf("removal of matching bmi did not remove all occurances, slice should have 5 items but has %v", len(u.sl))
	}

	u.remove(bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"bar1", "bar2", "bar3", "bar4"},
	})
	want = []bimodalIdentifier{bmi1, bmi1}
	if len(u.sl) != 2 {
		t.Fatalf("removal of matching bmi did not remove all occurances, slice should have 2 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
	}
}

func TestSelected(t *testing.T) {
	awp1 := atomWithPackages{
		a:  mkAtom("foo 1.0.0"),
		pl: []string{"foo1", "foo2", "foo3"},
	}
	awp1Pkgs := atomWithPackages{
		a:  mkAtom("foo 1.0.0"),
		pl: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
	}
	awp2 := atomWithPackages{
		a:  mkAtom("bar 1.0.0"),
		pl: []string{"bar1", "bar2"},
	}
	awp3 := atomWithPackages{
		a:  mkAtom("baz 1.0.0"),
		pl: []string{"baz1"},
	}
	awp4 := atomWithPackages{
		a:  mkAtom("bix 1.0.0"),
		pl: []string{"bix1", "bix2", "bix3"},
	}

	u := &selection{
		projects: []selected{
			{a: awp1, first: true},
			{a: awp2, first: true},
			{a: awp3, first: true},
			{a: awp4, first: true},
		},
		deps: make(map[ProjectRoot][]dependency),
	}

	u.pushSelection(awp1Pkgs, true)
	if u.Len() != 5 {
		t.Fatalf("len of unselected slice should have been 5 after pushing packages, got %v", u.Len())
	}
	lastProject := u.projects[len(u.projects)-1]
	if !reflect.DeepEqual(lastProject.a.pl, []string{"foo4", "foo5"}) {
		t.Fatal("the packages inserted overlapped with existing packages")
	}

	u.pushSelection(awp1, false)
	if u.Len() != 6 {
		t.Fatalf("len of unselected slice should have been 6 after pushing non-package atom, got %v", u.Len())
	}
}

func (u *unselected) compa(i int, j int) bool {
	return len(u.sl[i].pl) > len(u.sl[j].pl)
}

func TestNonexistentPopDep(t *testing.T) {
	// empty selector, has nothing in deps
	s := selection{}
	id := ProjectIdentifier{ProjectRoot: ProjectRoot("testroot"), Source: "testsource"}

	// previously this caused a panic if the id wasn't present in s.deps
	got := s.popDep(id)
	want := dependency{}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong item popped from deps:\n\t(GOT): %v\n\t(WANT): %v", got, want)
	}
}
