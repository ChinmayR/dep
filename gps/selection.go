// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"container/heap"
	"reflect"
)

type selection struct {
	// projects is a stack of the atoms that have currently been selected by the
	// solver. It can also be thought of as the vertex set of the current
	// selection graph.
	projects []selected
	// deps records the set of dependers on a given ProjectRoot. It is
	// essentially an adjacency list of *inbound* edges.
	deps map[ProjectRoot][]dependency
	// foldRoots records a mapping from a canonical, case-folded form of
	// ProjectRoots to the particular case variant that has currently been
	// selected.
	foldRoots map[string]ProjectRoot
	// The versoinUnifier in use for this solve run.
	vu *versionUnifier
}

type selected struct {
	a     atomWithPackages
	first bool
}

func (s *selection) Len() int {
	return len(s.projects)
}

func (s *selection) getDependenciesOn(id ProjectIdentifier) []dependency {
	if deps, exists := s.deps[id.ProjectRoot]; exists {
		return deps
	}

	return nil
}

// getIdentFor returns the ProjectIdentifier (so, the network name) currently in
// use for the provided ProjectRoot.
//
// If no dependencies are present yet that designate a network name for
// the provided root, this will return an empty ProjectIdentifier and false.
func (s *selection) getIdentFor(pr ProjectRoot) (ProjectIdentifier, bool) {
	deps := s.getDependenciesOn(ProjectIdentifier{ProjectRoot: pr})
	if len(deps) == 0 {
		return ProjectIdentifier{}, false
	}

	// For now, at least, the solver maintains (assumes?) the invariant that
	// whatever is first in the deps list decides the net name to be used.
	return deps[0].dep.Ident, true
}

// pushSelection pushes a new atomWithPackages onto the selection stack, along
// with an indicator as to whether this selection indicates a new project *and*
// packages, or merely some new packages on a project that was already selected.
func (s *selection) pushSelection(a atomWithPackages, pkgonly bool) {
	bmi := a.bmi()
	if pkgonly {
		// create a hash map of all the packages that are requested to be added
		foundPkgs := make(map[string]bool)
		for _, pkg := range bmi.pl {
			foundPkgs[pkg] = false
		}

		// check which packages that need to be added are already added
		for _, proj := range s.projects {
			pi := proj.a.bmi()
			if pi.id.eq(bmi.id) {
				for _, piPkg := range pi.pl {
					// if this package is in the foundPkgs and is not set to true, then set it to true
					if val, has := foundPkgs[piPkg]; has && !val {
						foundPkgs[piPkg] = true
					}
				}
			}
		}

		var pkgsToAdd []string
		for key, val := range foundPkgs {
			if !val {
				pkgsToAdd = append(pkgsToAdd, key)
			}
		}
		if len(pkgsToAdd) == 0 {
			return // all pkgs already exist in the selected queue
		}
		a.pl = pkgsToAdd
	}

	s.projects = append(s.projects, selected{
		a:     a,
		first: !pkgonly,
	})
}

// popSelection removes and returns the last atomWithPackages from the selection
// stack, along with an indication of whether that element was the first from
// that project - that is, if it represented an addition of both a project and
// one or more packages to the overall selection.
func (s *selection) popSelection() (atomWithPackages, bool) {
	var sel selected
	sel, s.projects = s.projects[len(s.projects)-1], s.projects[:len(s.projects)-1]
	return sel.a, sel.first
}

// findCaseConflicts checks to see if the given ProjectRoot has a
// case-insensitive overlap with another, different ProjectRoot that's already
// been picked.
func (s *selection) findCaseConflicts(pr ProjectRoot) (bool, ProjectRoot) {
	if current, has := s.foldRoots[toFold(string(pr))]; has && pr != current {
		return true, current
	}

	return false, ""
}

func (s *selection) pushDep(dep dependency) {
	pr := dep.dep.Ident.ProjectRoot
	deps := s.deps[pr]
	if len(deps) == 0 {
		s.foldRoots[toFold(string(pr))] = pr
	}

	for _, eachDep := range deps {
		if reflect.DeepEqual(eachDep, dep) {
			return
		}
	}

	s.deps[pr] = append(deps, dep)
}

func (s *selection) popDep(id ProjectIdentifier) (dep dependency) {
	deps := s.deps[id.ProjectRoot]
	dlen := len(deps)
	if dlen == 1 {
		delete(s.foldRoots, toFold(string(id.ProjectRoot)))
	}
	if dlen > 0 {
		dep, s.deps[id.ProjectRoot] = deps[dlen-1], deps[:dlen-1]
		return dep
	}
	// the specified project was not a key for the deps map, so there's nothing to pop back, return an empty dependency
	return dependency{}
}

func (s *selection) depperCount(id ProjectIdentifier) int {
	return len(s.deps[id.ProjectRoot])
}

// Compute a list of the unique packages within the given ProjectIdentifier that
// have dependers, and the number of dependers they have.
func (s *selection) getRequiredPackagesIn(id ProjectIdentifier) map[string]int {
	// TODO(sdboyer) this is horribly inefficient to do on the fly; we need a method to
	// precompute it on pushing a new dep, and preferably with an immut
	// structure so that we can pop with zero cost.
	uniq := make(map[string]int)
	for _, dep := range s.deps[id.ProjectRoot] {
		for _, pkg := range dep.dep.pl {
			uniq[pkg] = uniq[pkg] + 1
		}
	}

	return uniq
}

// Suppress unused warning.
var _ = (*selection)(nil).getSelectedPackagesIn

// Compute a list of the unique packages within the given ProjectIdentifier that
// are currently selected, and the number of times each package has been
// independently selected.
func (s *selection) getSelectedPackagesIn(id ProjectIdentifier) map[string]int {
	// TODO(sdboyer) this is horribly inefficient to do on the fly; we need a method to
	// precompute it on pushing a new dep, and preferably with an immut
	// structure so that we can pop with zero cost.
	uniq := make(map[string]int)
	for _, p := range s.projects {
		if p.a.a.id.eq(id) {
			for _, pkg := range p.a.pl {
				uniq[pkg] = uniq[pkg] + 1
			}
		}
	}

	return uniq
}

func (s *selection) getConstraint(id ProjectIdentifier) Constraint {
	deps, exists := s.deps[id.ProjectRoot]
	if !exists || len(deps) == 0 {
		return any
	}

	// TODO(sdboyer) recomputing this sucks and is quite wasteful. Precompute/cache it
	// on changes to the constraint set, instead.

	// The solver itself is expected to maintain the invariant that all the
	// constraints kept here collectively admit a non-empty set of versions. We
	// assume this is the case here while assembling a composite constraint.

	// Start with the open set
	var ret Constraint = any
	for _, dep := range deps {
		ret = s.vu.intersect(id, ret, dep.dep.Constraint)
	}

	return ret
}

// selected checks to see if the given ProjectIdentifier has been selected, and
// if so, returns the corresponding atomWithPackages.
//
// It walks the projects selection list from front to back and returns the first
// match it finds, which means it will always and only return the base selection
// of the project, without any additional package selections that may or may not
// have happened later.
func (s *selection) selected(id ProjectIdentifier) (atomWithPackages, bool) {
	for _, p := range s.projects {
		if p.a.a.id.ProjectRoot == id.ProjectRoot {
			return p.a, true
		}
	}

	return atomWithPackages{a: nilpa}, false
}

type unselected struct {
	sl  []bimodalIdentifier
	cmp func(i, j int) bool
}

func (u unselected) Len() int {
	return len(u.sl)
}

func (u unselected) Less(i, j int) bool {
	return u.cmp(i, j)
}

func (u unselected) Swap(i, j int) {
	u.sl[i], u.sl[j] = u.sl[j], u.sl[i]
}

func (u *unselected) Push(x interface{}) {
	// guard against pushing identical bmis into the unselected queue
	bmi := x.(bimodalIdentifier)
	plen := len(bmi.pl)
outer:
	for _, pi := range u.sl {
		if pi.id.eq(bmi.id) && len(pi.pl) == plen {
			// Simple slice comparison - assume they're both sorted the same
			for i, pkg := range pi.pl {
				if bmi.pl[i] != pkg {
					// found a package that doesn't match, continue looking
					continue outer
				}
			}
			// if all packages match, return without pushing
			return
		}
	}
	u.sl = append(u.sl, x.(bimodalIdentifier))
}

func (u *unselected) Pop() (v interface{}) {
	v, u.sl = u.sl[len(u.sl)-1], u.sl[:len(u.sl)-1]
	return v
}

// remove takes a bimodalIdentifier out of the priority queue, if present. All
// the matching bmis will be removed.
//
// There are two events that cause this to be called: bmi selection, when the
// bmi at the front of the queue is removed, and backtracking, when a bmi
// becomes unnecessary because the dependency that induced it was backtracked
// and popped off.
//
// The worst case for both of these is O(n), but in practice the first case is
// O(1), as we iterate the queue from front to back.
func (u *unselected) remove(bmi bimodalIdentifier) {
	plen := len(bmi.pl)
	numDeleted := 0
outer:
	for i := 0; i-numDeleted < len(u.sl); i++ {
		pi := u.sl[i-numDeleted]
		if pi.id.eq(bmi.id) && len(pi.pl) == plen {
			// Simple slice comparison - assume they're both sorted the same
			for i2, pkg := range pi.pl {
				if bmi.pl[i2] != pkg {
					continue outer
				}
			}

			heap.Remove(u, i-numDeleted)
			numDeleted++
		}
	}
}
