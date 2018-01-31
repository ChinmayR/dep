package telemetry

// MetricsVersion describes which telescope RFC this code adhears to.
// It is used as a tag on M3 metrics to help limit queries and avoid older
// metrics with known bugs or extreme cardinality.
// https://docs.google.com/document/d/1uG6EwaBxBJRCz-sJVbiNo716zYgqhDZigX-BZdjtKAI/
const MetricsVersion = "v1"
