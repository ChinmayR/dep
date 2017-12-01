Release process
===============

This document outlines how to create a release of the version library

1.  `git checkout master`

2.  `git pull`

3.  Alter CHANGELOG.md from `v<version>-dev (unreleased)` to
    `v<version_to_release> (YYYY-MM-DD)`

4.  Create a commit with the title `Preparing for release <version_to_release>`

5.  Create a git tag for the version using
    `git tag -a v<version_to_release> -m v<version_to_release` (e.g.
    `git tag -a v1.0.0 -m v1.0.0`)

6.  Push the tag to origin `git push --tags origin v<version_to_release>`

7. Update `CHANGELOG.md` to have a new `v<version>-dev (unreleased)` and
    put into a commit with title `Back to development`

8. `git push origin master`
