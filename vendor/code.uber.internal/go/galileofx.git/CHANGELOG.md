# Releases

## v1.3.0 (2017-12-18)

- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import the package.
- Add galileohttp package providing re-usable Galileo middleware for HTTP
  servers and clients.
- galileofx now provides HTTP client and server middleware to the container.

## v1.2.0 (2017-10-16)

- Galileo may now be disabled in production by setting `enabled: false`.
- Dropped direct dependency on Wonka.

## v1.1.0 (2017-09-19)

- Update to a stable release of the Wonka client library. No changes to
  exported functionality or API.

## v1.0.1 (2017-09-15)

- Fix a bug which attempted to authenticate requests to the Meta::health
  procedure.

## v1.0.0 (2017-09-06)

No changes since v0.2.0. This release is committing to making no breaking
changes to the current API in the 1.X series.

## v0.2.0 (2017-08-31)

- Raise the lower bound for Galileo to v1.1.

## v0.1.0 (2017-08-29)

- Initial release.
