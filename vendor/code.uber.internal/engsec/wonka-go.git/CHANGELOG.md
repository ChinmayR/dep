# Changelog

## v1.6.0 (2018-01-18)

- Added WonkaClientCertPath and WonkaClientKeyPath config variables.
- Removed dependency on "go.uber.org/atomic" package.
- Fixed an issue that could cause wonka to refresh certificates too often.
- Claims for AD groups are no longer case-sensitive (wonkamaster change).

## v1.5.1 (2017-12-22)

- Fix bug where wonkad would refresh certificates too often.

## v1.5.0 (2017-12-07)

- Add testdata.EnrollEntity helper for directly adding entities to Wonka's
  database.
- Augment claim validation errors with reason.
- Start during panic: Wonka client will start even when globally disabled.
- Return after panic: Wonka client can now return to normal functional mode
  after global disable.
- Fix issue where Wonka client can start without a private key.
- Don't ussh sign resolve requests.

## v1.4.5 (2017-11-30)
- Skip unit tests altogether for build debian step

## v1.4.4 (2017-11-30)
- Skip unit tests failing in production

## v1.4.3 (2017-11-23)

- Fix bug loading keys from secrets.yaml
- Add host:global to M3 metrics, and move version tag from M3 to logs.
- Fix bug so claim.Inspect now works as described and allows claims with one of
  multiple destinations.

## v1.4.2 (2017-11-14)

- Init will now succeed while globally disabled.
- Local testing for global disable, aka panic button.

## v1.4.1 (2017-11-08)

- add more tests

## v1.4.0 (2017-11-03)

- deprecate the Sign, Verify, Encrypt & Decrypt methods on the wonka interface.
- cut down logspam.
- more better tests. massive coverage improvement.
- upgrade everything to wonka certs.
- fix entity name == cert.entityname.
- add cookies (replacement for claims).
- added wonkamaster pubkey and url to config options (instead of using env vars).
- fix security flaw in ussh cert checking
- move claims package to internal (non-public).
- wonkamaster uses staging key when in staging environment.

## v1.3.1 (2017-10-26)

- Remove dependency on fx ^1 by internalizing envfx because UberFx-beta and Glue-beta
  are not compatible with fx ^1.

## v1.3.0 (2017-10-17)

- better errors from wonkad.
- remove wonkabar.uber.com, wonka-services require cerberus to test from outside of prod.
- better test coverage.
- respect the panic button in updateDerelicts()
- try to work around langley bug in loadKey()
- fix --generate-keys option for wonkacli
- update gobuild

## v1.2.0 (2017-10-05)

- better test coverage.
- cancel the background goroutines (cert refresh, derelict checking, globally disabled).
- /thehose now sends a signed request.
- globally disabled is now base32 encoded.

## v1.1.2 (2017-09-26)
- Remove derelict goroutine to mitigate live-site. See https://outages.uberinternal.com/incidents/586a638f-a49c-4d27-9fd8-2c442d46a36e.

## v1.1.1 (2017-09-25)

- omitempty for implicitclaims

## v1.1.0 (2017-09-22)

- implicit claims
- add EntityCrypter

## v1.0.1 (2017-09-20)

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

## v0.16.1 (2017-09-11)

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
