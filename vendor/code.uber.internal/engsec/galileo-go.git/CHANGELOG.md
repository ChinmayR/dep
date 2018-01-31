# Changelog

## v1.7.0 (2018-01-22)

- Log when authenticated entity name and unauthenticated caller name are different.
- Change name of M3 metric that indicates Wonka is globally disabled. No metric
  when Galileo is disabled by local configuration.
- Add `galileo.CallerName` option to `AuthenticateIn` and `ValidateCredential`
- `AuthenticateHTTPRequest` makes use of `galileo.CallerName`
- Logs and metrics from `AuthenticateIn` indicate when caller is considered
  derelict.
- Logs and metrics from `AuthenticateIn` now use more specific values for
  `unauthorized_reason`
- Add dynamic `SetConfig` API with `EnforcePercentage` option
- Upgrade to wonka-go v1.6

## v1.6.0 (2017-12-13)

- Don't validated cached outbound claims.
- Add `galileo.GetCredential` and `galileo.ValidateCredential`
- `galileotest.WithServerGalileo` and `galileotest.WithClientGalileo` now
  default to a mock Jaeger tracer.
- Add caching of inbound claim validation, with `cache.max_size` config.

## v1.5.0 (2017-12-07)

- Instantiating Galileo no longer attempts to enroll an entity.
- Log inbound authentication errors at `info` instead of `warn`.
- Add `servicealiases` configuration parameter allowing services to accept
  tokens with one of multiple allowed destinations.
- Add `galileotest.NewDisabled` to help customers with unit testing.
- GetClaim and AuthenticateIn now remove Wonka token from baggage to prevent
  leaking outside Uber or to other entities. (Wonka tokens are bearer tokens.)
- Add metricsversion:v1 tag to M3 metrics.
- Add yab request interceptor implementation.

## v1.4.0 (2017-11-14)

- AuthenticateOut returns an unmodified context without error when it cannot get
  a Wonka token.
- Improve observability by adding logs and M3 metrics to AuthenticateIn and
  AuthenticateOut.

## v1.3.4 (2017-11-08)

- bump wonka v1.4.1

## v1.3.3 (2017.09-26)

- ensure wonka never falls below 1.1.2

## v1.3.2 (2017-09-26)

- Always create a new Jaeger span for outbound authenticated requests.
  Fixes T1163179
- pull in no derelict checking hotfix

## v1.3.1 (2017-09-25)

- bump to get implicit claims bug fix.

## v1.3.0 (2017-09-25)

- Add `galileotest` package containing `MockGalileo` and helpful wrappers for
  integration testing.
- Stop using cached claim tokens that expire soon. Fixes T1172027

## v1.2.6 (2017-09-19)

- wonka ^1

## v1.2.5-rc1 (2017-09-15)

- wonka v1.0.0-rc1

## v1.2.4 (2017-09-15)

- undo pinning to unf\*ck glide

## v1.2.3 (2017-09-14)

- wonka v1.0.0-rc1

## v1.2.2 (2017-09-12)

- expose GetLogger() to return the logger associated with this galileo instance.
- add WithClaim to allow higher level apis (eg, uberfx) support setting explicit claims without using the variadic explicitClaim
- accidentally skipped 1.2.1

## v1.2.0 (2017-09-06)

- Support Wonka 0.16.
- Fixed bug which caused two log traces to be emitted for the same value.

## v1.1.0 (2017-08-31)

- Create and finish a span called 'galileo' during AuthenticateIn and
  AuthenticateOut if none is provided in context.
- Annotate spans with logs instead of tags to reduce Jaeger database cost.
- Add option to construct Galileo in disabled mode so AuthenticateIn and
  AuthenticateOut do not add claim tokens to baggage.
- `galileo.Configuration` now accepts an explicit tracer instead of always
  using the global tracer. The global tracer will still be used if this option
  is not specified.

## v1.0.2 (2017-08-29)

- bump to pull in wonka-go 15.2 which uses the timeouts specified in the passed-in context.

## v1.0.1 (2017-08-28)

- Propagate context into Wonka calls to maintain Jaeger traces.

## v1.0.0 (2017-08-23)

- cut a stable 1.0 (!)

## v0.15.0 (2017-08-23)

- Remove the `Galileo.Logger()` method.
- Rename the `AllowedEntities` function to `GetAllowedEntities`.
- Remove logger from `GetClaim` method arguments.
- `AuthenticateIn` now accepts the list of entities as variadic arguments
  rather than `[]string`.

## v0.14.0 (2017-08-23)

- Remove `GetOriginatingPrincipal`, `GetInboundAuth`, and `GetClaims` because we
  no longer support claim chains. Expose `GetClaim` instead.

## v0.13.0 (2017-08-23)

- remove go subpackage. everything is directly under galileo-go

## v0.12.0 (2017-08-23)

- update wonka (0.14 to 0.14.1)
- Remove `DropFailure` configuration parameter in favor of `EnforcePercentage`.
- Actually set the endpoint configuration.
- Set tags on inbound and outbound jaeger spans.

## v0.11.0 (2017-08-18)

- Remove `GetAttribute` and `SetAttribute` functions.

## v0.10.0 (2017-08-17)

- Remove `Galileo.NewWrappedServer`.
- Remove `EnforcePercentage` method. The `EnforcePercentage` is applied by the
  Galileo client internally and does not need to be enforced by the caller as
  well.
- Remove `AllowedEntities` method. Use the top-level `AllowedEntities` function
  on the error message to determine this information.

## v0.9.0 (2017-08-16)

- Add `AllowedEntities` and `EnforcePercentage` methods on `Galileo`.
- Remove `Galileo.AuthenticateEndpoint` method in favor of the
  `AuthenticateHTTPRequest` function.
- Remove `Galileo.NewHandler` in favor of go-common's `xhttp.NewGalileoFilter`.
- Remove `Galileo.NewClient` in favor of go-common's
  `xhttp.NewGalileoClientFilter`.
- Remove `Galileo.Router`.

## v0.8.0 (2017-08-10)

- change Authenticate, Authorize and AuthorizeEndpiont to AuthenticateOut,
  AuthenticateIn and AuthenticateEndpoint.

## v0.7.3 (2017-08-9)

- Use Zap for logging. APIs which accepted go-common's `log.Logger` now expect
  a `*zap.SugaredLogger`.

## v0.5.1 (2017-07-20)

- First tagged release
