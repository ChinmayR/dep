# Changelog

## v1.1.0 (2017-08-22)

- Always setup the global OpenTracing tracer even if nothing else consumes the
  tracer directly.
- In development and tests, configure Jaeger to log all spans.

## v1.0.0 (2017-07-31)

- No changes since previous release.

## v1.0.0-rc2 (2017-07-21)

- Export constructor as `New`.
- Update lifecycle hooks to satisfy new Fx APIs.
- Fixed a bug where baggage would not propagate in dev, or if tracing was
  disabled.

## v1.0.0-rc1 (2017-06-21)

- First public release.
