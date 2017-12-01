namespace java com.uber.health

enum State {
    REFUSING = 0,
    ACCEPTING = 1,
    STOPPING = 3,
    STOPPED = 4,
}

enum RequestType {
    LEGACY = 0,
    TRAFFIC = 1,
}

struct HealthRequest {
    1: optional RequestType type
}

struct HealthStatus {
    1: required bool ok
    2: optional string message
    3: optional State state
}

service Meta {
    HealthStatus health(1: HealthRequest hr)
}
