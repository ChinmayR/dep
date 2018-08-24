// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"fmt"
	"os"
	"strings"

	"github.com/golang/dep/uber"
)

type failedVersion struct {
	v Version
	f error
}

type versionQueue struct {
	id           ProjectIdentifier
	pi           []Version
	lockv, prefv Version
	fails        []failedVersion
	b            sourceBridge
	failed       bool
	allLoaded    bool
	adverr       error
}

func newVersionQueue(id ProjectIdentifier, lockv, prefv Version, b sourceBridge, constraint Constraint) (*versionQueue, error) {
	uber.DebugLogger.Printf("newVersionQueue %v, lockedV: %v, prefV: %v\n", id, lockv, prefv)
	vq := &versionQueue{
		id: id,
		b:  b,
	}

	// Lock goes in first, if present
	if lockv != nil {
		vq.lockv = lockv
		vq.pi = append(vq.pi, lockv)
	}

	// Preferred version next
	if prefv != nil {
		vq.prefv = prefv
		vq.pi = append(vq.pi, prefv)
	}

	if len(vq.pi) == 0 {
		var err error
		allVersions, err := vq.b.listVersions(vq.id)
		if err != nil {
			// TODO(sdboyer) pushing this error this early entails that we
			// unconditionally deep scan (e.g. vendor), as well as hitting the
			// network.
			return nil, err
		}
		vq.pi = filterNonDefaultBranches(allVersions, constraint, vq.id.ProjectRoot)
		vq.allLoaded = true
	}

	uber.DebugLogger.Printf("Version queue created for %v is %v", vq.id.ProjectRoot, vq.pi)
	return vq, nil
}

//This function filters versions according to these criteria:
//1. The version is a branch version that is not a default branch version – a default branch version has a hash that matches that of HEAD's
//2. There is not a constraint defined on either the branch or the semver version
//3. The semver version does not have a prerelease tag associated with it
//4. More than 5 versions have already been chosen
func filterNonDefaultBranches(allVersions []Version, constraint Constraint, root ProjectRoot) []Version {
	if os.Getenv(uber.UseNonDefaultVersionBranches) == "yes" {
		return allVersions
	}

	const VERSION_QUEUE_MAX_LIMIT = 5

	var filteredVersions []Version
	for _, version := range allVersions {
		if version.Type() == IsBranch {
			bv := version.(versionPair).v.(branchVersion)
			if bv.isDefault || (constraint != nil && !isAnyConstraint(constraint) && constraint.Matches(bv)) {
				filteredVersions = append(filteredVersions, version)
			}
		} else if version.Type() == IsSemver {
			// ignore all semver tags with a prerelease tag such as v1.0.0-rc9 or v1.0.0-beta1
			sv := version.(versionPair).v.(semVersion)
			semverMatchesConstraint := constraint != nil && !isAnyConstraint(constraint) && constraint.Matches(sv)
			if semverMatchesConstraint || (len(filteredVersions) < VERSION_QUEUE_MAX_LIMIT && strings.TrimSpace(sv.sv.Prerelease()) == "") {
				filteredVersions = append(filteredVersions, version)
			}
		}
	}
	if len(allVersions) == len(filteredVersions) {
		uber.UberLogger.Printf("No non-default branch versions found. \n\tprojectRoot=[%s] \n\twas=%s \n\tnow=%s", root, allVersions, filteredVersions)
	} else {
		uber.UberLogger.Printf("Branch version filtration complete. \n\tprojectRoot=[%s] \n\twas=%s \n\tnow=%s", root, allVersions, filteredVersions)
	}
	return filteredVersions
}

func isAnyConstraint(constraint Constraint) bool {
	_, isAnyConstraint := constraint.(anyConstraint)
	return isAnyConstraint
}

func (vq *versionQueue) current() Version {
	if len(vq.pi) > 0 {
		return vq.pi[0]
	}

	return nil
}

// advance moves the versionQueue forward to the next available version,
// recording the failure that eliminated the current version.
func (vq *versionQueue) advance(fail error, constraint Constraint, root ProjectRoot) error {
	// Nothing in the queue means...nothing in the queue, nicely enough
	if vq.adverr != nil || len(vq.pi) == 0 { // should be a redundant check, but just in case
		return vq.adverr
	}

	// Record the fail reason and pop the queue
	vq.fails = append(vq.fails, failedVersion{
		v: vq.pi[0],
		f: fail,
	})
	vq.pi = vq.pi[1:]

	// *now*, if the queue is empty, ensure all versions have been loaded
	if len(vq.pi) == 0 {
		if vq.allLoaded {
			// This branch gets hit when the queue is first fully exhausted,
			// after a previous advance() already called ListVersions().
			return nil
		}
		vq.allLoaded = true

		var vltmp []Version
		vltmp, vq.adverr = vq.b.listVersions(vq.id)
		if vq.adverr != nil {
			return vq.adverr
		}
		vltmp = filterNonDefaultBranches(vltmp, constraint, root)

		// defensive copy - calling listVersions here means slice contents may
		// be modified when removing prefv/lockv.
		vq.pi = make([]Version, len(vltmp))
		copy(vq.pi, vltmp)

		// search for and remove lockv and prefv, in a pointer GC-safe manner
		//
		// could use the version comparator for binary search here to avoid
		// O(n) each time...if it matters
		var delkeys []int
		for k, pi := range vq.pi {
			if pi == vq.lockv || pi == vq.prefv {
				delkeys = append(delkeys, k)
			}
		}

		for k, dk := range delkeys {
			dk -= k
			copy(vq.pi[dk:], vq.pi[dk+1:])
			// write nil to final position for GC safety
			vq.pi[len(vq.pi)-1] = nil
			vq.pi = vq.pi[:len(vq.pi)-1]
		}

		if len(vq.pi) == 0 {
			// If listing versions added nothing (new), then return now
			return nil
		}
	}

	// We're finally sure that there's something in the queue. Remove the
	// failure marker, as the current version may have failed, but the next one
	// hasn't yet
	vq.failed = false

	// If all have been loaded and the queue is empty, we're definitely out
	// of things to try. Return empty, though, because vq semantics dictate
	// that we don't explicitly indicate the end of the queue here.
	return nil
}

// isExhausted indicates whether or not the queue has definitely been exhausted,
// in which case it will return true.
//
// It may return false negatives - suggesting that there is more in the queue
// when a subsequent call to current() will be empty. Plan accordingly.
func (vq *versionQueue) isExhausted() bool {
	if !vq.allLoaded {
		return false
	}
	return len(vq.pi) == 0
}

func (vq *versionQueue) String() string {
	var vs []string

	for _, v := range vq.pi {
		vs = append(vs, v.String())
	}
	return fmt.Sprintf("[%s]", strings.Join(vs, ", "))
}
