package wonka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/internal/mocks/mock_redswitch"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"

	"github.com/golang/mock/gomock"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"gopkg.in/yaml.v2"
)

func TestCertificateRegistry(t *testing.T) {
	require.NotNil(t, _globalCertificateRegistry, "_globalCertificateRegistry should not be nil")

	setCertRefresher := func(f func(*certificateRepository, repositoryMapKey, time.Duration, *zap.Logger)) (restore func()) {
		old := _certRefresher
		_certRefresher = f
		restore = func() { _certRefresher = old }
		return
	}

	t.Run("register_dupes", func(t *testing.T) {
		h := newHTTPRequester(mocktracer.New(), zap.NewNop())
		h.writeURL("http://something")
		e := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
			},
			key:       nil,
			requester: h,
			log:       zap.L(),
		}

		refreshDone := make(chan struct{})
		registerDone := make(chan struct{})
		defer setCertRefresher(func(r *certificateRepository, k repositoryMapKey, period time.Duration, _ *zap.Logger) {
			<-registerDone
			defer close(refreshDone)
			require.NotNil(t, r)
			require.Equal(t, e.toKey(), k)
			require.Equal(t, certRefreshPeriod, period, "expected period to default to certRefreshPeriod")

			r.RLock()
			defer r.RUnlock()
			re := r.loadForRefresh(k)
			require.NotNil(t, re)
			require.Equal(t, 2, r.m[k].handles.Len())
		})()

		r := newCertificateRegistry()
		require.NotNil(t, r)

		handle, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle)

		handle2, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle2)

		require.Equal(t, handle.current, handle2.current)
		require.Equal(t, handle.mapKey, handle2.mapKey)

		registerDone <- struct{}{}
		<-refreshDone
	})
	t.Run("register_unique", func(t *testing.T) {
		e1 := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       zap.L(),
		}
		e2 := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "bar",
				Type:       EntityTypeService,
				Host:       "somehost",
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       zap.L(),
		}
		e3 := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeUser,
				Host:       "somehost",
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       zap.L(),
		}

		c := make(chan struct{}, 3)
		defer setCertRefresher(func(r *certificateRepository, k repositoryMapKey, period time.Duration, _ *zap.Logger) {
			require.Equal(t, certRefreshPeriod, period, "expected period to default to certRefreshPeriod")
			c <- struct{}{}
			if len(c) == cap(c) {
				close(c)
			}
		})()

		r := newCertificateRegistry()
		require.NotNil(t, r)

		handle1, err := r.Register(e1)
		require.NoError(t, err, "failed to register e1")
		require.NotNil(t, handle1)

		handle2, err := r.Register(e2)
		require.NoError(t, err, "failed to register e2")
		require.NotNil(t, handle2)

		handle3, err := r.Register(e3)
		require.NoError(t, err, "failed to register e3")
		require.NotNil(t, handle3)

		// Ensure that 3 goroutines are started
		<-c
		<-c
		<-c
	})
	t.Run("unregister", func(t *testing.T) {
		e := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
				Serial:     uint64(1),
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       zap.L(),
		}

		newpk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		registered := make(chan struct{}) // blocks until we've registered all entities
		updated := make(chan struct{})    // blocks until we've updated all handles
		defer setCertRefresher(func(r *certificateRepository, k repositoryMapKey, period time.Duration, _ *zap.Logger) {
			<-registered
			r.Lock()
			defer r.Unlock()
			e := r.m[k]
			newCurrent := certficateKeyTuple{
				cert: &Certificate{
					EntityName: "foo",
					Type:       EntityTypeService,
					Host:       "somehost",
					Serial:     uint64(2),
					Tags:       map[string]string{"updated": "true"},
				},
				key: newpk,
			}
			e.current = newCurrent
			handles := getHandles(e.handles)
			t.Logf("Updating %d handles", len(handles))
			go func() {
				updateHandles(handles, e.current)
				close(updated)
			}()
		})()

		r := newCertificateRegistry()
		require.NotNil(t, r)

		handle, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle)

		handle2, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle2)

		require.False(t, handle == handle2, "handles should not be the same pointer")
		close(registered)

		// Wait until we update the cert/key for all handles
		<-updated
		cert, key := handle.GetCertificateAndPrivateKey()
		require.Contains(t, cert.Tags, "updated")
		require.Equal(t, key, newpk)

		cert2, key2 := handle2.GetCertificateAndPrivateKey()
		require.Equal(t, cert, cert2)
		require.Equal(t, key, key2)

		require.NoError(t, r.Unregister(handle))
		require.NotEmpty(t, r.(*certificateRepository).m)
		require.NoError(t, r.Unregister(handle2))
		require.Empty(t, r.(*certificateRepository).m)
		require.EqualError(t, r.Unregister(handle), "failed to unregister instance because it was not in the registry")
	})
	t.Run("periodic_refresh", func(t *testing.T) {
		msgChan := make(chan string, 100)
		hooks := zap.Hooks(func(e zapcore.Entry) error {
			t.Log(e.Message)
			msgChan <- e.Message
			return nil
		})

		l, err := zap.Config{
			Level:         zap.NewAtomicLevelAt(zap.DebugLevel),
			Development:   true,
			Encoding:      "console",
			EncoderConfig: zap.NewDevelopmentEncoderConfig(),
		}.Build(hooks)
		require.NoError(t, err, "failed to create logger")

		e := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       l,
		}

		r := newCertificateRegistry()
		require.NotNil(t, r)

		handle, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle)

		go periodicWonkaCertRefresh(r.(*certificateRepository), e.toKey(), time.Millisecond, l)

		// Wait for our error messages to come which exercise the
		// attempt to refresh code path.
		var wmErr bool
		var wdErr bool
		for !wmErr || !wdErr {
			select {
			case m := <-msgChan:
				switch m {
				case "error refreshing from wonkamaster":
					wmErr = true
				case "error refreshing from wonkad":
					wdErr = true
				}
			}
		}

		require.NoError(t, r.Unregister(handle))

		// Wait for the goroutine to finish now that we've unregistered
		for {
			select {
			case m := <-msgChan:
				if m == "Terminating periodic certificate refresh" {
					return
				}
			}
		}
	})
	t.Run("update_entry", func(t *testing.T) {
		e := certificateRegistrationRequest{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
				Serial:     uint64(1),
			},
			key:       nil,
			requester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
			log:       zap.L(),
		}

		newpk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		defer setCertRefresher(func(_ *certificateRepository, _ repositoryMapKey, _ time.Duration, _ *zap.Logger) {
		})()

		r := newCertificateRegistry()
		handle, err := r.Register(e)
		require.NoError(t, err)
		require.NotNil(t, handle)

		newCurrent := certficateKeyTuple{
			cert: &Certificate{
				EntityName: "foo",
				Type:       EntityTypeService,
				Host:       "somehost",
				Serial:     uint64(2),
				Tags:       map[string]string{"updated": "true"},
			},
			key: newpk,
		}
		cr := r.(*certificateRepository)
		cr.updateEntry(e.toKey(), newCurrent)

		cr.Lock()
		require.Equal(t, newCurrent, cr.m[e.toKey()].current)
		cr.Unlock()

		// Updating a non-existant entry should not block anything
		cr.updateEntry(repositoryMapKey{}, newCurrent)

		// The handle should eventually be updated asynchronously
		nCert := newCurrent.cert
		for cert, key := handle.GetCertificateAndPrivateKey(); cert != nCert || key != newpk; cert, key = handle.GetCertificateAndPrivateKey() {
			time.Sleep(time.Millisecond)
		}
	})
}

func BenchmarkIsDerelict(b *testing.B) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, ln.Close(), "listener failed to close")
	}()

	mux := http.NewServeMux()
	expire, err := time.Parse("2006-01-02", "2020-01-01")
	require.NoError(b, err)

	rep := TheHoseReply{
		CurrentStatus: "ok",
		CurrentTime:   int64(time.Now().Unix()),
		CheckInterval: 300,
		Derelicts:     map[string]time.Time{"foober": expire},
	}
	encoded, err := json.Marshal(&rep)
	require.NoError(b, err)

	mux.HandleFunc(string(hoseEndpoint), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", xhttp.MIMETypeApplicationJSON)
		w.Write(encoded)
	})
	s := http.Server{Handler: mux}
	go s.Serve(ln)

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(b, err)
	w := &uberWonka{
		entityName:             "foobar",
		log:                    zap.L(),
		clientECC:              k,
		derelictsRefreshPeriod: internal.DerelictsCheckPeriod,
		derelictsTimer:         time.NewTimer(time.Nanosecond),
		metrics:                tally.NoopScope,
		httpRequester:          newHTTPRequester(mocktracer.New(), zap.NewNop()),
	}

	w.httpRequester.writeURL(fmt.Sprintf("http://%s", ln.Addr().String()))
	require.NoError(b, w.updateDerelicts(context.Background()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		assert.True(b, w.IsDerelict("foober"))
	}
}

func TestRefreshDerelicts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, ln.Close(), "listener failed to close")
	}()

	refreshChan := make(chan int64, 1)
	mux := http.NewServeMux()
	expire, err := time.Parse("2006-01-02", "2020-01-01")
	require.NoError(t, err)
	mux.HandleFunc(string(hoseEndpoint), func(w http.ResponseWriter, r *http.Request) {
		rep := TheHoseReply{
			CurrentStatus: "ok",
			CurrentTime:   int64(time.Now().Unix()),
			CheckInterval: 300,
			Derelicts:     map[string]time.Time{"foober": expire},
		}
		xhttp.RespondWithJSON(w, rep)
		refreshChan <- rep.CurrentTime
	})
	s := http.Server{Handler: mux}
	go s.Serve(ln)

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	mc := gomock.NewController(t)
	defer mc.Finish()

	r := mock_redswitch.NewMockCachelessReader(mc)
	r.EXPECT().IsDisabled().Return(false)

	w := &uberWonka{
		entityName:             "foobar",
		log:                    zap.L(),
		clientECC:              k,
		derelictsRefreshPeriod: internal.DerelictsCheckPeriod,
		derelictsTimer:         time.NewTimer(0),
		globalDisableReader:    r,
		httpRequester:          newHTTPRequester(mocktracer.New(), zap.NewNop()),
		metrics:                tally.NoopScope,
	}

	w.httpRequester.writeURL(fmt.Sprintf("http://%s", ln.Addr().String()))

	// Loop until we refresh by the timer firing
	refreshed := false
	for !refreshed {
		select {
		case <-refreshChan:
			refreshed = true
		default:
			w.IsDerelict("foober")
		}
	}

	// We know at this point that the timer has fired, but everything may not be updated
	// yet. Let it settle (or timeout here if something is wrong)
	for !w.IsDerelict("foober") {
		time.Sleep(time.Millisecond)
	}
}

func TestIsDerelictWithInvalidWonka(t *testing.T) {
	require.False(t, IsDerelict(nil, "entity"))
}

func TestUpdateDerelicts(t *testing.T) {
	var testCases = []struct {
		name string
		run  func(*testing.T, *uberWonka)
	}{
		{
			"valid",
			func(t *testing.T, u *uberWonka) {
				ctx := context.Background()
				err := u.updateDerelicts(ctx)
				require.NoError(t, err)
				require.Equal(t, 300*time.Second, u.derelictsRefreshPeriod)
				require.Contains(t, u.derelicts, "foober")
			},
		},
		{
			"signing_error",
			func(t *testing.T, u *uberWonka) {
				u.clientECC = nil
				ctx := context.Background()
				err := u.updateDerelicts(ctx)
				require.Error(t, err, "error signing request")
			},
		},
		{
			"bad_url",
			func(t *testing.T, u *uberWonka) {
				u.httpRequester.writeURL("")
				ctx := context.Background()
				err := u.updateDerelicts(ctx)
				require.Error(t, err, "unsupported protocol")
			},
		},
		{
			"is_derelict",
			func(t *testing.T, u *uberWonka) {
				ctx := context.Background()
				err := u.updateDerelicts(ctx)
				require.NoError(t, err)
				require.True(t, u.IsDerelict("foober"))
				require.False(t, u.IsDerelict("none"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, ln.Close(), "listener failed to close")
			}()

			mux := http.NewServeMux()
			expire, err := time.Parse("2006-01-02", "2020-01-01")
			require.NoError(t, err)
			mux.HandleFunc(string(hoseEndpoint), func(w http.ResponseWriter, r *http.Request) {
				rep := TheHoseReply{
					CurrentStatus: "ok",
					CurrentTime:   int64(time.Now().Unix()),
					CheckInterval: 300,
					Derelicts:     map[string]time.Time{"foober": expire},
				}
				xhttp.RespondWithJSON(w, rep)
			})
			s := http.Server{Handler: mux}
			go s.Serve(ln)

			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)

			mc := gomock.NewController(t)
			defer mc.Finish()

			r := mock_redswitch.NewMockCachelessReader(mc)
			r.EXPECT().IsDisabled().Return(false)

			w := &uberWonka{
				entityName:             "foobar",
				log:                    zap.L(),
				clientECC:              k,
				derelictsRefreshPeriod: internal.DerelictsCheckPeriod,
				derelictsTimer:         time.NewTimer(time.Hour),
				globalDisableReader:    r,
				httpRequester:          newHTTPRequester(mocktracer.New(), zap.NewNop()),
				metrics:                tally.NoopScope,
			}

			w.httpRequester.writeURL(fmt.Sprintf("http://%s", ln.Addr().String()))
			tc.run(t, w)
		})
	}
}

func TestCSRSignWithSSH(t *testing.T) {
	var testVars = []struct {
		name   string
		noKeys bool
		err    string
	}{{name: "foober"},
		{name: "foober", noKeys: true, err: "no ussh certs found"},
		{name: "foober"}}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			withSSHAgent(m.name, func(a agent.Agent) {
				if m.noKeys {
					a.RemoveAll()
				}

				cert, _, err := NewCertificate(CertEntityName(m.name))
				require.NoError(t, err)

				certSig := &CertificateSignature{
					Certificate: *cert,
					Timestamp:   int64(time.Now().Unix()),
					Data:        []byte("I'm a little teapot"),
				}

				csr, err := signCSRWithSSH(cert, certSig, a, zap.L())
				if m.err == "" {
					require.NoError(t, err)
					require.NotNil(t, csr)
				} else {
					require.Nil(t, csr)
					require.Error(t, err)
					require.Contains(t, err.Error(), m.err)
				}
			})
		})
	}
}

func TestCSRSignWithCert(t *testing.T) {
	var testVars = []struct {
		name          string
		noCert        bool
		noSigningCert bool

		err string
	}{
		{name: "foober"},
		{name: "foober", noCert: true, err: "certificate is nil"},
		{name: "foober", noSigningCert: true, err: "nil cert"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			entityCert, entityPrivKey, err := NewCertificate(CertEntityName(m.name))
			require.NoError(t, err)
			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			err = entityCert.SignCertificate(k)
			require.NoError(t, err)

			oldKeys := WonkaMasterPublicKeys
			WonkaMasterPublicKeys = []*ecdsa.PublicKey{&k.PublicKey}
			defer func() { WonkaMasterPublicKeys = oldKeys }()

			if m.noSigningCert {
				entityCert = nil
			}

			cert, _, err := NewCertificate(CertEntityName(m.name))
			require.NoError(t, err)

			if m.noCert {
				cert = nil
			}

			csr, err := signCSRWithCert(cert, entityCert, entityPrivKey)
			if m.err == "" {
				require.NoError(t, err)
				require.NotNil(t, csr)
			} else {
				require.Nil(t, csr)
				require.Error(t, err)
				require.Contains(t, err.Error(), m.err)
			}
		})
	}
}

func TestTheHose(t *testing.T) {
	w := &uberWonka{
		log:                    zap.L(),
		derelicts:              make(map[string]time.Time),
		derelictsRefreshPeriod: internal.DerelictsCheckPeriod,
		derelictsTimer:         time.NewTimer(time.Hour),
	}

	ok := IsDerelict(w, "")
	require.False(t, ok, "empty should be false")

	ok = IsDerelict(w, "foo")
	require.False(t, ok, "should not be a derelict")

	w.derelicts["foo"] = time.Now().Add(-time.Hour)
	ok = IsDerelict(w, "foo")
	require.False(t, ok, "should not be a derelict")

	w.derelicts["foo"] = time.Now().Add(24 * time.Hour)
	ok = IsDerelict(w, "foo")
	require.True(t, ok, "should be a derelict")
}

func TestDisabled(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		mc := gomock.NewController(t)
		defer mc.Finish()

		r := mock_redswitch.NewMockCachelessReader(mc)
		r.EXPECT().IsDisabled().Return(false)
		w := &uberWonka{
			globalDisableReader: r,
		}
		ok := IsGloballyDisabled(w)
		require.False(t, ok)
	})
	t.Run("disabled", func(t *testing.T) {
		mc := gomock.NewController(t)
		defer mc.Finish()

		r := mock_redswitch.NewMockCachelessReader(mc)
		r.EXPECT().IsDisabled().Return(true)
		w := &uberWonka{
			globalDisableReader: r,
		}
		ok := IsGloballyDisabled(w)
		require.True(t, ok)
	})
}

func TestURL(t *testing.T) {
	var testVars = []struct {
		expires   time.Duration
		shouldErr bool
	}{
		{expires: 500 * time.Millisecond, shouldErr: false},
		{expires: 0, shouldErr: true},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			mux := http.NewServeMux()
			mux.HandleFunc(string(healthEndpoint), func(w http.ResponseWriter, r *http.Request) {
				xhttp.RespondWithJSON(w, GenericResponse{Result: "OK"})
			})
			s := http.Server{Handler: mux}
			go s.Serve(ln)

			urls := make(chan string, 1)
			ctx, cancel := context.WithTimeout(context.Background(), m.expires)
			defer cancel()

			hr := newHTTPRequester(mocktracer.New(), zap.NewNop())
			prober := httpProber{
				client: hr.client,
				log:    zap.NewNop(),
			}

			url := fmt.Sprintf("http://%s", ln.Addr())
			go prober.Do(ctx, urls, url)

			u := <-urls
			if m.shouldErr {
				require.Empty(t, u)
			} else {
				require.Equal(t, url, u)
			}
		})
	}
}

func TestHttpRequesterConcurrent(t *testing.T) {
	h := newHTTPRequester(mocktracer.New(), zap.NewNop())
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		h.SetURL(context.Background(), "http://test")
	}()

	go func() {
		defer wg.Done()
		in := struct{}{}
		out := GenericResponse{}
		h.Do(context.Background(), healthEndpoint, in, &out)
	}()

	wg.Wait()
}

func TestSetURL(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	defer testhelper.UnsetEnvVar("WONKA_MASTER_HOST")()
	defer testhelper.UnsetEnvVar("WONKA_MASTER_PORT")()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc(string(healthEndpoint), func(w http.ResponseWriter, r *http.Request) {
		xhttp.RespondWithJSON(w, GenericResponse{Result: "OK"})
	})
	s := http.Server{Handler: mux}
	go s.Serve(ln)

	w := &uberWonka{
		log:           zap.NewNop(),
		httpRequester: newHTTPRequester(mocktracer.New(), zap.NewNop()),
	}
	ctx := context.Background()
	oldURLS := _wmURLS

	func() {
		defer testhelper.UnsetEnvVar("UBER_DATACENTER")()
		_wmURLS = []string{}
		w.wonkaURLRequested = ""
		err = w.httpRequester.SetURL(ctx, w.wonkaURLRequested)
		require.Error(t, err)
	}()

	url := fmt.Sprintf("http://%s", ln.Addr())
	_wmURLS = []string{url}
	w.wonkaURLRequested = ""
	err = w.httpRequester.SetURL(ctx, w.wonkaURLRequested)
	require.NoError(t, err)
	require.Equal(t, w.httpRequester.URL(), _wmURLS[0])

	_wmURLS = []string{}
	w.wonkaURLRequested = url
	err = w.httpRequester.SetURL(ctx, w.wonkaURLRequested)
	require.NoError(t, err)
	require.Equal(t, w.httpRequester.URL(), url)

	defer testhelper.SetEnvVar("WONKA_MASTER_URL", url)()
	w.wonkaURLRequested = ""
	err = w.httpRequester.SetURL(ctx, w.wonkaURLRequested)
	require.NoError(t, err)
	require.Equal(t, w.httpRequester.URL(), url)

	_wmURLS = oldURLS
}

func TestCertificateEqual(t *testing.T) {
	c, _, err := NewCertificate(CertEntityName("foober"))
	require.NoError(t, err)

	marshalled, err := MarshalCertificate(*c)
	require.NoError(t, err)

	unmarshalled, err := UnmarshalCertificate(marshalled)
	require.NoError(t, err)

	require.True(t, c.equal(unmarshalled))
}

func withSSHAgent(name string, fn func(agent.Agent)) {
	a := agent.NewKeyring()
	authority, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	signer, err := ssh.NewSignerFromKey(authority)
	if err != nil {
		panic(err)
	}

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	pubKey, err := ssh.NewPublicKey(&k.PublicKey)
	if err != nil {
		panic(err)
	}

	cert := &ssh.Certificate{
		CertType:        ssh.UserCert,
		Key:             pubKey,
		Serial:          1,
		ValidPrincipals: []string{name},
		ValidAfter:      0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
	}

	if err := cert.SignCert(rand.Reader, signer); err != nil {
		panic(err)
	}

	if err := a.Add(agent.AddedKey{PrivateKey: k}); err != nil {
		panic(err)
	}

	if err := a.Add(agent.AddedKey{PrivateKey: k, Certificate: cert}); err != nil {
		panic(err)
	}

	fn(a)
}

func TestLoadKeyAndUpgrade(t *testing.T) {
	createWonka := func(disabled bool, mc *gomock.Controller) *uberWonka {
		w := &uberWonka{
			log:                 zap.NewNop(),
			metrics:             tally.NoopScope,
			globalDisableReader: mock_redswitch.NewMockCachelessReader(mc),
		}

		if disabled {
			w.globalDisableReader.(*mock_redswitch.MockCachelessReader).EXPECT().IsDisabled().AnyTimes().Return(true)
		} else {
			w.globalDisableReader.(*mock_redswitch.MockCachelessReader).EXPECT().IsDisabled().AnyTimes().Return(false)
		}

		return w
	}

	// Load from a static key
	f, err := ioutil.TempFile("", "pem")
	require.NoError(t, err, "failed to create temp file")
	defer func() { os.Remove(f.Name()) }()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "failed to generate rsa private key")

	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	require.NoError(t, pem.Encode(f, pemBlock), "failed to encode test pem file")

	cfg := Config{
		PrivateKeyPath: f.Name(),
	}

	t.Run("wonka_disabled", func(t *testing.T) {
		mc := gomock.NewController(t)
		defer mc.Finish()
		w := createWonka(true, mc)
		require.NoError(t, w.loadKeyAndUpgrade(context.Background(), cfg),
			"failure to upgrade key should not result in an error when wonka is globally disabled")
	})
	t.Run("wonka_enabled", func(t *testing.T) {
		mc := gomock.NewController(t)
		defer mc.Finish()
		w := createWonka(false, mc)
		require.Error(t, w.loadKeyAndUpgrade(context.Background(), cfg),
			"failure to upgrade key should result in an error when wonka is globally disabled")
	})
}

func TestLoadKeyFromPath(t *testing.T) {
	t.Run("secrets_yaml", func(t *testing.T) {
		type LangleyYAML struct {
			Private string `yaml:"wonka_private"`
		}

		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err, "failed to generate test key")

		b := x509.MarshalPKCS1PrivateKey(key)

		// Base 64 encode
		b64 := base64.StdEncoding.EncodeToString(b)

		// Strip off header and footer
		b64 = strings.TrimPrefix(b64, rsaPrivHeader)
		b64 = strings.TrimSuffix(b64, rsaPrivFooter)

		// Save to YAML
		dir, err := ioutil.TempDir("", "wonka_test")
		require.NoError(t, err, "failed to create temp directory")
		defer os.Remove(dir)

		ly := LangleyYAML{Private: b64}
		data, err := yaml.Marshal(&ly)
		require.NoError(t, err, "failed to marshal")

		t.Logf("Marshalled to %s", data)

		path := filepath.Join(dir, "secrets.yaml")
		err = ioutil.WriteFile(path, data, 0666)
		require.NoError(t, err, "failed to write test file")

		w := uberWonka{
			log: zap.NewNop(),
		}
		loaded, err := w.loadKeyFromPath(path)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		require.Equal(t, key, loaded)
	})
}

func TestInitRedswitchReader(t *testing.T) {
	t.Run("fail_to_create", func(*testing.T) {
		w := &uberWonka{}
		require.Error(t, w.initRedswitchReader())
	})
	t.Run("valid", func(*testing.T) {
		w := &uberWonka{
			log:                   zap.NewNop(),
			metrics:               tally.NoopScope,
			globalDisableRecovery: make(chan time.Time),
		}
		require.NoError(t, w.initRedswitchReader())
	})
}

func TestLoadKeysUsingConfig(t *testing.T) {
	dir, err := ioutil.TempDir("", "testwonka")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// create cert and key files
	keyFoo := `MHcCAQEEIPvqzWVtm+AswUkb4O+SHZr5TCKZjv954JMlcY8xpMGboAoGCCqGSM49AwEHoUQDQgAEzv4TPL4tCg2t5BaIUJjWjjiFZCQ69htnXcxR4e8tj0jHgxNjeeP1nyV4f017TZlvQVm2/P5q1s9t+UxGStfmYA==`
	certFileFoo := filepath.Join(dir, "cert-foo")
	keyFileFoo := filepath.Join(dir, "key-foo")
	createFile(t, certFileFoo, `{"entity_name": "foo"}`)
	createFile(t, keyFileFoo, keyFoo)

	// mock out the disable reader for the IsDisabled() check
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockDisableReader := mock_redswitch.NewMockCachelessReader(mockCtrl)
	mockDisableReader.EXPECT().IsDisabled().Return(true)

	tests := []struct {
		description    string
		cfg            Config
		expectedEntity string
		expectedKey    string
		expectedError  string
	}{
		{
			description:   "no paths",
			expectedError: "Client cert and/or client key are not set in wonka.Config",
		},
		{
			description:    "config paths",
			expectedEntity: "foo",
			expectedKey:    keyFoo,
			cfg: Config{
				WonkaClientCertPath: certFileFoo,
				WonkaClientKeyPath:  keyFileFoo,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			w := &uberWonka{
				log:                 zap.L(),
				globalDisableReader: mockDisableReader,
			}
			err = w.loadCertAndKeyFromConfig(context.Background(), tt.cfg)
			if err != nil {
				require.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedEntity, w.certificate.EntityName)
			require.Equal(t, tt.expectedKey, marshalECCToString(t, w.clientECC))
		})
	}
}

func createFile(t *testing.T, filename, data string) {
	err := ioutil.WriteFile(filename, []byte(data), 0777)
	require.NoError(t, err)
}

func marshalECCToString(t *testing.T, key *ecdsa.PrivateKey) string {
	b, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}
