package httpfx_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"code.uber.internal/go/httpfx.git/httpserver"
	"code.uber.internal/go/httpfx.git/servermiddleware"
	"go.uber.org/fx"
)

// NewServeMux builds an http.ServeMux which contains all the handlers for our
// server.
//
// For this example, we're using ServeMux but there's nothing stopping us from
// using gorilla/mux.Router or another HTTP multiplexer since they all
// implement http.Handler.
func NewServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Received request to /hello")
		io.WriteString(w, "hello to you too")
	})
	return mux
}

// ServerParams defines the different objects we need to build our
// http.Server.
type ServerParams struct {
	fx.In

	// Mux is the http.ServeMux our server will serve.
	Mux *http.ServeMux

	// WrapJaeger is the middleware provided by jaegerfx that adds Jaeger
	// tracing support to a handler.
	WrapJaeger func(http.Handler) http.Handler `name:"trace"`

	// WrapGalileo is the middleware provided by galileofx that adds Galileo
	// authentication support to our handler.
	WrapGalileo func(http.Handler) http.Handler `name:"auth"`
}

// NewServer builds an HTTP server for our service.
func NewServer(p ServerParams) *http.Server {
	// We wrap the http.ServeMux with Galileo and Jaeger support. Note that
	// order here is VERY important. Gallieo relies on Jaeger so the Jaeger
	// middleware MUST run first.
	handler := servermiddleware.Chain(
		p.WrapJaeger,
		p.WrapGalileo,
		// Your middleware goes here.
	)(p.Mux)
	return &http.Server{
		// We're using a hard-coded port here but in a real application we'll
		// want to load this from the YAML configuration using a
		// config.Provider.
		Addr:    ":9999",
		Handler: handler,
	}
}

// StartServer schedules our HTTP server to be started when the Fx application
// attempts to start.
func StartServer(server *http.Server, lc fx.Lifecycle) {
	// We build a Handle to the HTTP server and tell Fx how to start and stop
	// it.
	h := httpserver.NewHandle(server)
	lc.Append(fx.Hook{OnStart: h.Start, OnStop: h.Shutdown})
}

func Example_server() {
	// Usually this will be inside a main().
	app := fx.New(
		// In a real application, replace this line with uberfx.Module.
		FakeModule,

		// This tells Fx how to build our http.ServeMux and the http.Server.
		fx.Provide(
			NewServeMux,
			NewServer,
		),

		// This asks for the server to be started when we start the Fx app.
		fx.Invoke(StartServer),
	)

	// In a real application, all the code below this line will be replaced
	// with,
	//
	//   app.Run()

	// Start the application, waiting up to a second for it to start up.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		panic(err)
	}
	defer func() {
		// Attempt a graceful shutdown of the application, waiting a second before
		// giving up.
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := app.Stop(ctx); err != nil {
			panic(err)
		}
	}()

	// We'll make a request to this server for the test.
	res, err := http.Get("http://127.0.0.1:9999/hello")
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	msg, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Server said %q\n", msg)

	// Output:
	// Received request to /hello
	// Server said "hello to you too"
}
