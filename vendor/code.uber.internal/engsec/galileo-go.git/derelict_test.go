package galileo_test

import (
	"net/http"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDerelict(t *testing.T) {
	var testVars = []struct {
		desc string
		name string

		isDerelict bool
	}{
		{desc: "good standing", name: "shiny", isDerelict: false},
		{desc: "empty", name: "", isDerelict: false},
		{desc: "globally derelict", name: "crufty", isDerelict: true},
	}

	galileotest.WithServerGalileo(t, "server-under-test", func(g galileo.Galileo) {
		time.Sleep(1 * time.Second) // Wonka client loads derelict list asynchronously

		dg, ok := g.(galileo.DerelictGalileo)
		require.True(t, ok, "Galileo from WithServerGalileo should implement DerelictGalileo")

		for _, m := range testVars {
			t.Run(m.desc, func(t *testing.T) {

				t.Run("IsDerelictHttp", func(t *testing.T) {
					r := &http.Request{Header: make(map[string][]string)}
					r.Header.Set("x-uber-source", m.name)

					// this tests the real underlying wonka instance which will always
					// return false in our tests.
					ok := galileo.IsDerelictHttp(g, r)
					assert.Equal(t, m.isDerelict, ok, "expected %q to have derelict status %v", m.name, m.isDerelict)
				})

				t.Run("DerelictGalileo", func(t *testing.T) {

					ok := dg.IsDerelict(m.name)
					assert.Equal(t, m.isDerelict, ok, "expected %q to have derelict status %v", m.name, m.isDerelict)
				})
			})
		}
	},
		galileotest.GlobalDerelictEntities("crufty"),
		galileotest.EnrolledEntities("server-under-test"),
	)
}
