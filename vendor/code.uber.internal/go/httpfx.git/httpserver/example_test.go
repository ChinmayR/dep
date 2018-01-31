package httpserver_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"code.uber.internal/go/httpfx.git/httpserver"
)

func ExampleHandle() {
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "hello")
		}),
	}

	handle := httpserver.NewHandle(server)

	// handle.Start blocks until the server is ready to accept requests.
	if err := handle.Start(context.Background()); err != nil {
		panic(err)
	}

	url := fmt.Sprintf("http://%v/", handle.Addr())
	res, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Received: %s\n", body)

	if err := handle.Shutdown(context.Background()); err != nil {
		panic(err)
	}

	// Output: Received: hello
}
