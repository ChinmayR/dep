namespace java com.uber.go.ussocli

service UssoCli {
  HelloResponse hello(1:HelloRequest request);
}

struct HelloRequest {
    1: optional string name;
}

struct HelloResponse {
    1: optional string message;
}
