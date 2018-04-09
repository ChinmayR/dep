// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/golang/dep/uber"
)

// A multiCache manages two cache levels, ephemeral in-memory and persistent on-disk.
//
// The in-memory cache is always checked first, with the on-disk used as a fallback.
// Values read from disk are set in-memory when an appropriate method exists.
//
// Set values are cached both in-memory and on-disk.
type multiCache struct {
	mem, disk singleSourceCache
}

func (c *multiCache) setManifestAndLock(r Revision, ai ProjectAnalyzerInfo, m Manifest, l Lock) {
	uber.CacheLogger.Printf("executing multi setManifestAndLock, rev:%v, manifest:%v, lock:%v", r, m, l)
	c.mem.setManifestAndLock(r, ai, m, l)
	c.disk.setManifestAndLock(r, ai, m, l)
	uber.CacheLogger.Printf("successful multi setManifestAndLock")
}

func (c *multiCache) getManifestAndLock(r Revision, ai ProjectAnalyzerInfo) (Manifest, Lock, bool) {
	uber.CacheLogger.Printf("executing multi getManifestAndLock, rev:%v", r)
	m, l, ok := c.mem.getManifestAndLock(r, ai)
	if ok {
		uber.CacheLogger.Printf("hit getManifestAndLock from mem, rev:%v, manifest:%v, lock:%v", r, m, l)
		return m, l, true
	}

	m, l, ok = c.disk.getManifestAndLock(r, ai)
	if ok {
		uber.CacheLogger.Printf("hit getManifestAndLock from disk, rev:%v, manifest:%v, lock:%v", r, m, l)
		c.mem.setManifestAndLock(r, ai, m, l)
		return m, l, true
	}

	uber.CacheLogger.Printf("miss multi setManifestAndLock, rev:%v, manifest:%v, lock:%v", r, m, l)
	return nil, nil, false
}

func (c *multiCache) setPackageTree(r Revision, ptree pkgtree.PackageTree) {
	uber.CacheLogger.Printf("executing multi setPackageTree, rev:%v, pkgTree:%v", ptree)
	c.mem.setPackageTree(r, ptree)
	c.disk.setPackageTree(r, ptree)
	uber.CacheLogger.Printf("successful multi setPackageTree, rev:%v, pkgTree:%v", ptree)
}

func (c *multiCache) getPackageTree(r Revision) (pkgtree.PackageTree, bool) {
	uber.CacheLogger.Printf("executing multi getPackageTree, rev:%v", r)
	ptree, ok := c.mem.getPackageTree(r)
	if ok {
		uber.CacheLogger.Printf("hit getPackageTree from mem, rev:%v, pkgTree:%v", r, ptree)
		return ptree, true
	}

	ptree, ok = c.disk.getPackageTree(r)
	if ok {
		uber.CacheLogger.Printf("hit getPackageTree from disk, rev:%v, pkgTree:%v", r, ptree)
		c.mem.setPackageTree(r, ptree)
		return ptree, true
	}

	uber.CacheLogger.Printf("miss multi getPackageTree, rev:%v", r)
	return pkgtree.PackageTree{}, false
}

func (c *multiCache) markRevisionExists(r Revision) {
	uber.CacheLogger.Printf("executing multi markRevisionExists, rev:%v", r)
	c.mem.markRevisionExists(r)
	c.disk.markRevisionExists(r)
	uber.CacheLogger.Printf("successful multi markRevisionExists, rev:%v", r)
}

func (c *multiCache) setVersionMap(pvs []PairedVersion) {
	uber.CacheLogger.Printf("executing multi setVersionMap, pairedVersionMap:%v", pvs)
	c.mem.setVersionMap(pvs)
	c.disk.setVersionMap(pvs)
	uber.CacheLogger.Printf("successful multi setVersionMap, pairedVersionMap:%v", pvs)
}

func (c *multiCache) getVersionsFor(rev Revision) ([]UnpairedVersion, bool) {
	uber.CacheLogger.Printf("executing multi getVersionsFor, rev:%v", rev)
	uvs, ok := c.mem.getVersionsFor(rev)
	if ok {
		uber.CacheLogger.Printf("hit getVersionsFor from mem, rev:%v, unpairedVersion:%v", rev, uvs)
		return uvs, true
	}

	uber.CacheLogger.Printf("miss multi getVersionsFor, rev:%v", rev)
	return c.disk.getVersionsFor(rev)
}

func (c *multiCache) getAllVersions() ([]PairedVersion, bool) {
	uber.CacheLogger.Printf("executing multi getAllVersions")
	pvs, ok := c.mem.getAllVersions()
	if ok {
		uber.CacheLogger.Printf("hit getAllVersions from mem, unpairedVersion:%v", pvs)
		return pvs, true
	}

	pvs, ok = c.disk.getAllVersions()
	if ok {
		uber.CacheLogger.Printf("hit getAllVersions from disk, unpairedVersion:%v", pvs)
		c.mem.setVersionMap(pvs)
		return pvs, true
	}

	uber.CacheLogger.Printf("miss multi getAllVersions")
	return nil, false
}

func (c *multiCache) getRevisionFor(uv UnpairedVersion) (Revision, bool) {
	uber.CacheLogger.Printf("executing multi getRevisionFor, unpairedVersion:%v", uv)
	rev, ok := c.mem.getRevisionFor(uv)
	if ok {
		uber.CacheLogger.Printf("hit getRevisionFor from mem, unpairedVersion:%v, rev:%v", uv, rev)
		return rev, true
	}

	uber.CacheLogger.Printf("miss multi getRevisionFor, unpairedVersion:%v", uv)
	return c.disk.getRevisionFor(uv)
}

func (c *multiCache) toRevision(v Version) (Revision, bool) {
	uber.CacheLogger.Printf("executing multi toRevision, version:%v", v)
	rev, ok := c.mem.toRevision(v)
	if ok {
		uber.CacheLogger.Printf("hit multi toRevision from mem, version:%v, rev:%v", v, rev)
		return rev, true
	}

	uber.CacheLogger.Printf("miss multi toRevision, version:%v", v)
	return c.disk.toRevision(v)
}

func (c *multiCache) toUnpaired(v Version) (UnpairedVersion, bool) {
	uber.CacheLogger.Printf("executing multi toUnpaired, version:%v", v)
	uv, ok := c.mem.toUnpaired(v)
	if ok {
		uber.CacheLogger.Printf("hit multi toUnpaired from mem, version:%v, unpairedVersion:%c", v, uv)
		return uv, true
	}

	uber.CacheLogger.Printf("miss multi toUnpaired, version:%v", v)
	return c.disk.toUnpaired(v)
}
