This repository contains several components associated with wonka ecosystem:
1. `wonkamaster` http service for granting claim tokens
2. `wonka-go` client library to interact with `wonkamaster`
3. `wonkacli` command line tool to interact with `wonkamaster`

# wonkamaster

For details about wonkamaster see [wonkamaster/README.md]

# Release Process

1. Write code, `arc diff`, code review, `arc land`
2. Repeat 1 as many times as you want. A release can include any number of
   diffs.
3. Examine diffs since last release and decide what the new version should be.
   Follow Major.Minor.Patch format as defined by semver.org :
   * Bump major version for breaking changes, like when adding or removing
     from a defined interface.
   * Bump minor version for new, backward compatible, features.
   * Bump patch version for bug fixes. Upgrading to a patch release should
     never break a consumer.
4. Edit `version.go` by hand and enter the version you have chosen.
   Use only Major.Minor.Patch, removing the `-dev` label.
5. Execute `make release`
