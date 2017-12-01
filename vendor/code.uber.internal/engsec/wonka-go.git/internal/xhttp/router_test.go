package xhttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, h http.Handler) net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)

	go http.Serve(l, h)
	return l
}

func TestRouting(t *testing.T) {
	r := NewRouter()
	l := serve(t, r)
	defer l.Close()

	r.AddRoute(PathMatchesRegexp(regexp.MustCompile("/foo/(bar|zed)/quokka")),
		HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			RespondWithJSON(w, map[string]string{"route": "first"})
		}))
	r.AddPatternRoute("/foo/(ren|stimpy)/quokka",
		HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			RespondWithJSON(w, map[string]string{"route": "second"})
		}))

	var response map[string]string
	err := GetJSON(context.Background(), DefaultClient, fmt.Sprintf("http://%s/foo/ren/quokka", l.Addr().String()), &response, nil)
	require.Nil(t, err)
	assert.Equal(t, "second", response["route"])

	err = GetJSON(context.Background(), DefaultClient, fmt.Sprintf("http://%s/foo/bar/quokka", l.Addr().String()), &response, nil)
	require.Nil(t, err)
	assert.Equal(t, "first", response["route"])

	err = GetJSON(context.Background(), DefaultClient, fmt.Sprintf("http://%s/foo/unknown/quokka", l.Addr().String()), &response, nil)
	require.NotNil(t, err)
	errHTTP, ok := err.(ResponseError)
	require.True(t, ok)
	assert.Equal(t, 404, errHTTP.StatusCode)
}
