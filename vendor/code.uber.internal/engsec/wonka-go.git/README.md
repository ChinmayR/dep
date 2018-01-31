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
   Use only Major.Minor.Patch, leaving the `-dev` label.
5. Execute `make release`

## Additional information

See the [Releases Page](https://code.uberinternal.com/w/projects/security/galileo/runbooks/releases/)
for detailed release steps for each platform and library that depends on wonka-go.

## Packaging for Debian

1. Update the changelog `./debian/changelog`
2. Build wonkad/wonkacli on jenkins (https://ci.uberinternal.com/job/wonkad-deb/)
3. Find your updated package on a devbox/adhoc machine
   * `sudo apt-get update && sudo apt-cache madison wonkacli wonkad`
   * Your new package is listed as `<binary-name> | <deb-version>-<jenkins-build-number>.gbp<hash> | ...`
   * e.g. `wonkad | 1.0.2~22775.gbpb860a4 | ...`
   * Note: the version used is the one from `debian/changelog`
4. Update the package with `sudo apt-get install wonkacli=<deb-version>`
5. After sufficient testing/canarying, use the deb version name to update the version in puppet
   * https://code.uberinternal.com/diffusion/P/browse/master/hiera/os/Debian/jessie.yaml

## Testing return after panic

on a devserver:

1. run a local wonkamaster. copy the compressed ecc public key for later

2. generate a wonka cert. this is your signing cert. This is equivalent to the
   wonka certificate used by the mesos master to sign the launch request.

  sudo SSH_AUTH_SOCK="" WONKA_MASTER_ECC_PUB=<ecckey> ./wonkacli certificate -c wonka.cert -k wonka.key

3. use that certificate and key to generate a signed launch request. This is the signed
   launch request.

  sudo ./wonkacli task --certificate wonka.cert \
       --key wonka.key --sign \
       '{"hostname":"<your devserver name>", "svc_id":"<some service id>", "task_id":"<some task id>"}' > lr

4. use that launch request to generate a new wonka cert and key. This is equivalent
   to what the mesos agent on computeYY-sjc1 would be running.

  sudo ./wonkacli task -certificate new_wonka.cert -key new_wonka.key --cgc $(cat lr)

5. use new_wonka.cert and new_wonka.key to upgrade to real wonka certificate and key.
   This is equivalent to the nw service starting up with the return-after-panic cert.

  WONKA_MASTER_ECC_PUB=<ecckey> SSH_AUTH_SOCK="" WONKA_CLIENT_CERT=new_wonka.cert WONKA_CLIENT_KEY=new_wonka.key ./wonkacli -self <your task id> request -to foober -e 1m

you can do 2-5 by running the following:

```
#!/bin/sh

if [ $# -lt 2 ]; then
    echo "args"
    exit 1
fi

sudo SSH_AUTH_SOCK="" WONKA_MASTER_ECC_PUB=$1 ./wonkacli certificate -c wonka.cert -k wonka.key || exit 1

sudo ./wonkacli task --certificate wonka.cert --key wonka.key --sign '{"hostname":"REPLACE_HOSTNAME", "svc_id":"pmoody-test", "task_id":"pmoody-test-1234"}' > lr || exit 1

sudo ./wonkacli task -certificate new_wonka.cert -key new_wonka.key --cgc $(cat lr) || exit 1

sudo chmod 0644 wonka_new.cert wonka_new.key

WONKA_MASTER_ECC_PUB=$1 SSH_AUTH_SOCK="" WONKA_CLIENT_CERT=new_wonka.cert WONKA_CLIENT_KEY=new_wonka.key ./wonkacli -v -self $2 request -to foober -e 1m || exit 1

```
