// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"github.com/golang/dep/internal/gps/pkgtree"
)

// discardCache discards set values and returns nothing.
type discardCache struct{}

func (discardCache) setManifestAndLock(Revision, ProjectAnalyzerInfo, Manifest, Lock) {}

func (discardCache) getManifestAndLock(Revision, ProjectAnalyzerInfo) (Manifest, Lock, bool) {
	return nil, nil, false
}

func (discardCache) setPackageTree(Revision, pkgtree.PackageTree) {}

func (discardCache) getPackageTree(Revision) (pkgtree.PackageTree, bool) {
	return pkgtree.PackageTree{}, false
}

func (discardCache) markRevisionExists(r Revision) {}

func (discardCache) setVersionMap(versionList []PairedVersion) {}

func (discardCache) getVersionsFor(Revision) ([]UnpairedVersion, bool) {
	return nil, false
}

func (discardCache) getAllVersions() ([]PairedVersion, bool) {
	return nil, false
}

func (discardCache) getRevisionFor(UnpairedVersion) (Revision, bool) {
	return "", false
}

func (discardCache) toRevision(v Version) (Revision, bool) {
	return "", false
}

func (discardCache) toUnpaired(v Version) (UnpairedVersion, bool) {
	return nil, false
}
