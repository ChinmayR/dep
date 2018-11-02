// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/fs"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

// Example string to be written to the manifest file
// if no dependencies are found in the project
// during `dep init`
var exampleTOML = []byte(`# Gopkg.toml example
#
# Refer to https://golang.github.io/dep/docs/Gopkg.toml.html
# for detailed Gopkg.toml documentation.
#
# required = ["github.com/user/thing/cmd/thing"]
# ignored = ["github.com/user/project/pkgX", "bitbucket.org/user/project/pkgA/pkgY"]
#
# [[constraint]]
#   name = "github.com/user/project"
#   version = "1.0.0"
#
# [[constraint]]
#   name = "github.com/user/project2"
#   branch = "dev"
#   source = "github.com/myfork/project2"
#
# [[override]]
#   name = "github.com/x/y"
#   version = "2.4.0"
#
# [prune]
#   non-go = false
#   go-tests = true
#   unused-packages = true

`)

// String added on top of lock file
var lockFileComment = []byte(`# This file is autogenerated, do not edit; changes may be undone by the next 'dep ensure'.

`)

// SafeWriter transactionalizes writes of manifest, lock, and vendor dir, both
// individually and in any combination, into a pseudo-atomic action with
// transactional rollback.
//
// It is not impervious to errors (writing to disk is hard), but it should
// guard against non-arcane failure conditions.
type SafeWriter struct {
	Manifest     *Manifest
	lock         *Lock
	lockDiff     *gps.LockDiff
	writeVendor  bool
	writeLock    bool
	pruneOptions gps.CascadingPruneOptions
}

// NewSafeWriter sets up a SafeWriter to write a set of manifest, lock, and
// vendor tree.
//
// - If manifest is provided, it will be written to the standard manifest file
// name beneath root.
//
// - If newLock is provided, it will be written to the standard lock file
// name beneath root.
//
// - If vendor is VendorAlways, or is VendorOnChanged and the locks are different,
// the vendor directory will be written beneath root based on newLock.
//
// - If oldLock is provided without newLock, error.
//
// - If vendor is VendorAlways without a newLock, error.
func NewSafeWriter(manifest *Manifest, oldLock, newLock *Lock, vendor VendorBehavior, prune gps.CascadingPruneOptions) (*SafeWriter, error) {
	sw := &SafeWriter{
		Manifest:     manifest,
		lock:         newLock,
		pruneOptions: prune,
	}

	if oldLock != nil {
		if newLock == nil {
			return nil, errors.New("must provide newLock when oldLock is specified")
		}

		sw.lockDiff = gps.DiffLocks(oldLock, newLock)
		if sw.lockDiff != nil {
			sw.writeLock = true
		}
	} else if newLock != nil {
		sw.writeLock = true
	}

	switch vendor {
	case VendorAlways:
		sw.writeVendor = true
	case VendorOnChanged:
		sw.writeVendor = sw.lockDiff != nil || (newLock != nil && oldLock == nil)
	}

	if sw.writeVendor && newLock == nil {
		return nil, errors.New("must provide newLock in order to write out vendor")
	}

	return sw, nil
}

// HasLock checks if a Lock is present in the SafeWriter
func (sw *SafeWriter) HasLock() bool {
	return sw.lock != nil
}

// HasManifest checks if a Manifest is present in the SafeWriter
func (sw *SafeWriter) HasManifest() bool {
	return sw.Manifest != nil
}

type rawStringDiff struct {
	*gps.StringDiff
}

// MarshalTOML serializes the diff as a string.
func (diff rawStringDiff) MarshalTOML() ([]byte, error) {
	return []byte(diff.String()), nil
}

type rawLockedProjectDiff struct {
	Name     gps.ProjectRoot `toml:"name"`
	Source   *rawStringDiff  `toml:"source,omitempty"`
	Version  *rawStringDiff  `toml:"version,omitempty"`
	Branch   *rawStringDiff  `toml:"branch,omitempty"`
	Revision *rawStringDiff  `toml:"revision,omitempty"`
	Packages []rawStringDiff `toml:"packages,omitempty"`
}

func toRawLockedProjectDiff(diff gps.LockedProjectDiff) rawLockedProjectDiff {
	// this is a shallow copy since we aren't modifying the raw diff
	raw := rawLockedProjectDiff{Name: diff.Name}
	if diff.Source != nil {
		raw.Source = &rawStringDiff{diff.Source}
	}
	if diff.Version != nil {
		raw.Version = &rawStringDiff{diff.Version}
	}
	if diff.Branch != nil {
		raw.Branch = &rawStringDiff{diff.Branch}
	}
	if diff.Revision != nil {
		raw.Revision = &rawStringDiff{diff.Revision}
	}
	raw.Packages = make([]rawStringDiff, len(diff.Packages))
	for i := 0; i < len(diff.Packages); i++ {
		raw.Packages[i] = rawStringDiff{&diff.Packages[i]}
	}
	return raw
}

type rawLockedProjectDiffs struct {
	Projects []rawLockedProjectDiff `toml:"projects"`
}

func toRawLockedProjectDiffs(diffs []gps.LockedProjectDiff) rawLockedProjectDiffs {
	raw := rawLockedProjectDiffs{
		Projects: make([]rawLockedProjectDiff, len(diffs)),
	}

	for i := 0; i < len(diffs); i++ {
		raw.Projects[i] = toRawLockedProjectDiff(diffs[i])
	}

	return raw
}

func formatLockDiff(diff gps.LockDiff) (string, error) {
	var buf bytes.Buffer

	if diff.HashDiff != nil {
		buf.WriteString(fmt.Sprintf("Memo: %s\n\n", diff.HashDiff))
	}

	writeDiffs := func(diffs []gps.LockedProjectDiff) error {
		raw := toRawLockedProjectDiffs(diffs)
		chunk, err := toml.Marshal(raw)
		if err != nil {
			return err
		}
		buf.Write(chunk)
		buf.WriteString("\n")
		return nil
	}

	if len(diff.Add) > 0 {
		buf.WriteString("Add:")
		err := writeDiffs(diff.Add)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Add")
		}
	}

	if len(diff.Remove) > 0 {
		buf.WriteString("Remove:")
		err := writeDiffs(diff.Remove)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Remove")
		}
	}

	if len(diff.Modify) > 0 {
		buf.WriteString("Modify:")
		err := writeDiffs(diff.Modify)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Modify")
		}
	}

	return buf.String(), nil
}

// VendorBehavior defines when the vendor directory should be written.
type VendorBehavior int

const (
	// VendorOnChanged indicates that the vendor directory should be written when the lock is new or changed.
	VendorOnChanged VendorBehavior = iota
	// VendorAlways forces the vendor directory to always be written.
	VendorAlways
	// VendorNever indicates the vendor directory should never be written.
	VendorNever
)

func (sw SafeWriter) validate(root string, sm gps.SourceManager) error {
	if root == "" {
		return errors.New("root path must be non-empty")
	}
	if is, err := fs.IsDir(root); !is {
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return errors.Errorf("root path %q does not exist", root)
	}

	if sw.writeVendor && sm == nil {
		return errors.New("must provide a SourceManager if writing out a vendor dir")
	}

	return nil
}

// Write saves some combination of config yaml, lock, and a vendor tree.
// root is the absolute path of root dir in which to write.
// sm is only required if vendor is being written.
//
// It first writes to a temp dir, then moves them in place if and only if all the write
// operations succeeded. It also does its best to roll back if any moves fail.
// This mostly guarantees that dep cannot exit with a partial write that would
// leave an undefined state on disk.
//
// If logger is not nil, progress will be logged after each project write.
func (sw *SafeWriter) Write(root string, sm gps.SourceManager, examples bool, logger *log.Logger) error {
	err := sw.validate(root, sm)
	if err != nil {
		return err
	}

	if !sw.HasManifest() && !sw.writeLock && !sw.writeVendor {
		// nothing to do
		return nil
	}

	// if the separate lock digest exists then flag that it needs to be updated
	writeSeparateDigest := false
	ldpath := filepath.Join(root, LockDigest)
	if _, err := os.Stat(ldpath); err == nil {
		writeSeparateDigest = true
	}

	mpath := filepath.Join(root, ManifestName)
	lpath := filepath.Join(root, LockName)
	vpath := filepath.Join(root, "vendor")

	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if sw.HasManifest() {
		// Always write the example text to the bottom of the TOML file.
		tb, err := sw.Manifest.MarshalTOML()
		if err != nil {
			return errors.Wrap(err, "failed to marshal manifest to TOML")
		}

		var initOutput []byte

		// If examples are enabled, use the example text
		if examples {
			initOutput = exampleTOML
		}

		if err = ioutil.WriteFile(filepath.Join(td, ManifestName), append(initOutput, tb...), 0666); err != nil {
			return errors.Wrap(err, "failed to write manifest file to temp dir")
		}
	}

	if sw.writeLock {
		l, err := sw.lock.MarshalTOML(!writeSeparateDigest)
		if err != nil {
			return errors.Wrap(err, "failed to marshal lock to TOML")
		}

		if err = ioutil.WriteFile(filepath.Join(td, LockName), append(lockFileComment, l...), 0666); err != nil {
			return errors.Wrap(err, "failed to write lock file to temp dir")
		}

		if writeSeparateDigest {
			if err = ioutil.WriteFile(filepath.Join(td, LockDigest), []byte(hex.EncodeToString(sw.lock.SolveMeta.InputsDigest)), 0666); err != nil {
				return errors.Wrap(err, "failed to write lock digest file to temp dir")
			}
		}
	}

	if sw.writeVendor {
		var onWrite func(gps.WriteProgress)
		if logger != nil {
			onWrite = func(progress gps.WriteProgress) {
				logger.Println(progress)
			}
		}
		err = gps.WriteDepTree(filepath.Join(td, "vendor"), sw.lock, sm, sw.pruneOptions, onWrite)
		if err != nil {
			return errors.Wrap(err, "error while writing out vendor tree")
		}
	}

	// Ensure vendor/.git is preserved if present
	if hasDotGit(vpath) {
		err = fs.RenameWithFallback(filepath.Join(vpath, ".git"), filepath.Join(td, "vendor/.git"))
		if _, ok := err.(*os.LinkError); ok {
			return errors.Wrap(err, "failed to preserve vendor/.git")
		}
	}

	// Move the existing files and dirs to the temp dir while we put the new
	// ones in, to provide insurance against errors for as long as possible.
	type pathpair struct {
		from, to string
	}
	var restore []pathpair
	var failerr error
	var vendorbak string

	if sw.HasManifest() {
		if _, err := os.Stat(mpath); err == nil {
			// Move out the old one.
			tmploc := filepath.Join(td, ManifestName+".orig")
			failerr = fs.RenameWithFallback(mpath, tmploc)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: tmploc, to: mpath})
		}

		// Move in the new one.
		failerr = fs.RenameWithFallback(filepath.Join(td, ManifestName), mpath)
		if failerr != nil {
			goto fail
		}
	}

	if sw.writeLock {
		if _, err := os.Stat(lpath); err == nil {
			// Move out the old one.
			tmploc := filepath.Join(td, LockName+".orig")

			failerr = fs.RenameWithFallback(lpath, tmploc)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: tmploc, to: lpath})
		}

		// Move in the new one.
		failerr = fs.RenameWithFallback(filepath.Join(td, LockName), lpath)
		if failerr != nil {
			goto fail
		}

		// this moves the existing file to the temp dir while the new one is
		// put in the repo dir. the code provides a fail safe mechanism and
		// rolling back incase of an error
		if writeSeparateDigest {
			if _, err := os.Stat(ldpath); err == nil {
				// Move out the old one.
				tmploc := filepath.Join(td, LockDigest+".orig")

				failerr = fs.RenameWithFallback(ldpath, tmploc)
				if failerr != nil {
					goto fail
				}
				restore = append(restore, pathpair{from: tmploc, to: ldpath})
			}

			// Move in the new one.
			failerr = fs.RenameWithFallback(filepath.Join(td, LockDigest), ldpath)
			if failerr != nil {
				goto fail
			}
		}
	}

	if sw.writeVendor {
		if _, err := os.Stat(vpath); err == nil {
			// Move out the old vendor dir. just do it into an adjacent dir, to
			// try to mitigate the possibility of a pointless cross-filesystem
			// move with a temp directory.
			vendorbak = vpath + ".orig"
			if _, err := os.Stat(vendorbak); err == nil {
				// If the adjacent dir already exists, bite the bullet and move
				// to a proper tempdir.
				vendorbak = filepath.Join(td, ".vendor.orig")
			}

			failerr = fs.RenameWithFallback(vpath, vendorbak)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: vendorbak, to: vpath})
		}

		// Move in the new one.
		failerr = fs.RenameWithFallback(filepath.Join(td, "vendor"), vpath)
		if failerr != nil {
			goto fail
		}
	}

	// Renames all went smoothly. The deferred os.RemoveAll will get the temp
	// dir, but if we wrote vendor, we have to clean that up directly
	if sw.writeVendor {
		// Nothing we can really do about an error at this point, so ignore it
		os.RemoveAll(vendorbak)
	}

	return nil

fail:
	// If we failed at any point, move all the things back into place, then bail.
	for _, pair := range restore {
		// Nothing we can do on err here, as we're already in recovery mode.
		fs.RenameWithFallback(pair.from, pair.to)
	}
	return failerr
}

// PrintPreparedActions logs the actions a call to Write would perform.
func (sw *SafeWriter) PrintPreparedActions(output *log.Logger, verbose bool) error {
	if sw.HasManifest() {
		if verbose {
			m, err := sw.Manifest.MarshalTOML()
			if err != nil {
				return errors.Wrap(err, "ensure DryRun cannot serialize manifest")
			}
			output.Printf("Would have written the following %s:\n%s\n", ManifestName, string(m))
		} else {
			output.Printf("Would have written %s.\n", ManifestName)
		}
	}

	if sw.writeLock {
		if sw.lockDiff == nil {
			if verbose {
				l, err := sw.lock.MarshalTOML(true)
				if err != nil {
					return errors.Wrap(err, "ensure DryRun cannot serialize lock")
				}
				output.Printf("Would have written the following %s:\n%s\n", LockName, string(l))
			} else {
				output.Printf("Would have written %s.\n", LockName)
			}
		} else {
			output.Printf("Would have written the following changes to %s:\n", LockName)
			diff, err := formatLockDiff(*sw.lockDiff)
			if err != nil {
				return errors.Wrap(err, "ensure DryRun cannot serialize the lock diff")
			}
			output.Println(diff)
		}
	}

	if sw.writeVendor {
		if verbose {
			output.Printf("Would have written the following %d projects to the vendor directory:\n", len(sw.lock.Projects()))
			lps := sw.lock.Projects()
			for i, p := range lps {
				output.Printf("(%d/%d) %s@%s\n", i+1, len(lps), p.Ident(), p.Version())
			}
		} else {
			output.Printf("Would have written %d projects to the vendor directory.\n", len(sw.lock.Projects()))
		}
	}

	return nil
}

// hasDotGit checks if a given path has .git file or directory in it.
func hasDotGit(path string) bool {
	gitfilepath := filepath.Join(path, ".git")
	_, err := os.Stat(gitfilepath)
	return err == nil
}
