package debugfx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	systemportfx "code.uber.internal/go/systemportfx.git"
	"code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestRun(t *testing.T) {
	mux := http.NewServeMux()
	reporter := &versionfx.Reporter{}
	app := fxtest.New(t,
		fx.Provide(func() systemportfx.Mux { return mux }),
		fx.Provide(func() *versionfx.Reporter { return reporter }),
		Module,
	)
	app.RequireStart()
	defer app.RequireStop()

	assert.Equal(t, Version, reporter.Version(_name), "Version not reported for debugfx")
	response := requestMux(mux, "/debug/pprof/goroutine")
	assert.Equal(t, http.StatusOK, response.Code, "pprof handlers not registered")
}

func TestProfilingHandlers(t *testing.T) {
	mux := http.NewServeMux()
	registerPProf(mux)

	t.Run("unknown profile", func(t *testing.T) {
		response := requestMux(mux, "/debug/pprof/unknown")
		assert.Equal(t, http.StatusNotFound, response.Code, "Status code")
	})

	t.Run("success", func(t *testing.T) {
		suffixes := []string{"/", "/symbol", "/goroutine"}
		for _, suffix := range suffixes {
			response := requestMux(mux, "/debug/pprof"+suffix)
			assert.Equal(t, http.StatusOK, response.Code, "Status code")
		}
	})
}

func requestMux(mux *http.ServeMux, url string) *httptest.ResponseRecorder {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", url, nil)
	mux.ServeHTTP(rw, req)
	return rw
}
