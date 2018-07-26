// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"reflect"
	"testing"
)

// Regression test for https://github.com/sdboyer/gps/issues/174
func TestUnselectedRemoval(t *testing.T) {
	// We don't need a comparison function for this test
	bmi1 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo", "bar"},
	}
	bmi2 := bimodalIdentifier{
		id: mkPI("foo"),
		pl: []string{"foo", "bar", "baz"},
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
		sl: []bimodalIdentifier{bmi1, bmi2, bmi3, bmi4},
	}
	u.cmp = u.compa

	u.remove(bimodalIdentifier{
		id: mkPI("other"),
		pl: []string{"other"},
	})

	if len(u.sl) != 4 {
		t.Fatalf("len of unselected slice should have been 3 after no-op removal, got %v", len(u.sl))
	}

	u.remove(bmi3)
	want := []bimodalIdentifier{bmi4, bmi2, bmi1}
	if len(u.sl) != 3 {
		t.Fatalf("removal of matching bmi did not work, slice should have 2 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
	}

	u.remove(bmi3)
	if len(u.sl) != 3 {
		t.Fatalf("removal of bmi w/non-matching packages should be a no-op but wasn't; slice should have 2 items but has %v", len(u.sl))
	}

	u.remove(bmi2)
	want = []bimodalIdentifier{bmi4, bmi1}
	if len(u.sl) != 2 {
		t.Fatalf("removal of matching bmi did not work, slice should have 1 items but has %v", len(u.sl))
	}
	if !reflect.DeepEqual(u.sl, want) {
		t.Fatalf("wrong item removed from slice:\n\t(GOT): %v\n\t(WNT): %v", u.sl, want)
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
