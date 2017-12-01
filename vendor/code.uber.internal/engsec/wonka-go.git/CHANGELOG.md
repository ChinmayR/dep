# Changelog

## v1.2.0-dev (2017-10-05)

- better test coverage.
- cancel the background goroutines (cert refresh, derelict checking, globally disabled).
- /thehose now sends a signed request.
- globally disabled is now base32 encoded.

## v1.1.2 (2017-09-26)
- Remove derelict goroutine to mitigate live-site. See https://outages.uberinternal.com/incidents/586a638f-a49c-4d27-9fd8-2c442d46a36e. 

## v1.1.1-dev (2017-09-25)

- omitempty for implicitclaims

## v1.1.0 (2017-09-22)

- implicit claims
- add EntityCrypter

## v1.0.1-dev (2017-09-20)

- bug fix when checking for derelicts.

## v1.0.0 (2017-09-18)

- continue to support old rsa key
- remove 20ms ping timeout
- link to explanation when we change the entity name
- proxy to wonkamaster through cerberus
- fix wonkacli enroll with pem bug

## v1.0.0-rc1 (2017-09-13)

- Remove Wonka.Claim{Check,Validate,Inspect} to reduce surface of Wonka
  interface.
- Rename Claim.Valid to Claim.Validate.
- Claim.{Check,Inspect,Validate} should return a maximally descriptive error not
  bool so we don't have to deal with providing a logger.
- Remove package level wonka.ClaimCheck from public api.
- Add wonka.IsGloballyDisabled.
- Fix impersonated claim requests.

## v0.16.1-dev (2017-09-11)

- add staging configuration
- a number of client segfault fixes
- include deployment and taskid on refreshed certs
- more wonkamaster tests
- better metrics and logging
- derelict checking directly in wonka-go
- force the entity name when using ussh
- multiple wonkamaster keys

## v0.16.0 (2017-09-05)

- make enroll work for services again
- add build information for wonkacli
- add status handler for wonkamaster
- remove globals
- resolve endpoint for wonkamaster

## v0.15.3 (2017-08-31)

- Make enroll great again.
- Fix segfault during claim request when entity does not exist.
- Use the passed in context for lookup requests.
- Add an option to export an extra x509 file for private key.

## v0.15.2 (2017-08-29)

- use the passed in context for lookup and claim request.

## v0.15.1 (2017-08-28)

- remove timeout on lookup and claim request.

## v0.15.0 (2017-08-25)

- Switch to standard library's context package instead of using net/context.
  This drops Wonka's dependency on golang.org/x/net/context.
- Pass context throughout.
- Add WonkaMasterURL to config
- Add NullEntity. Nothing matches the null entity.

## v0.14.2 (2017-08-22)

- remove all galileo from wonkamaster

## v0.14.1 (2017-08-17)

- wonkamaster: fix tchannel peer list
- wonka-go: refresh certs against wonkamaster

## v0.14.0 (2017-08-17)

- xhttp: Don't use AllowedEntities/EnforcePercentage

## v0.13.0 (2017-08-17)

- support for galileo 0.10

## v0.13.0 (2017-08-16)

- Remove dependency on go-common.
- wonkatestdata.WithWonkaMaster now expects a function which accepts a common.Router.

## v0.12.0 (2017-08-15)

- Accept `*zap.Logger` instead of `*zap.SugaredLogger`

## v0.11.1 (2017-08-11)

- certificate backed claim requests.

## v0.11.0 (2017-08-10)

- update galileo

## v0.10.0 (2017-08-09)

- Use Zap for logging. APIs which accepted go-common's `log.Logger` now expect
  a `*zap.SugaredLogger`.

## v0.9.6 (2017-07-20)

- First tagged release
