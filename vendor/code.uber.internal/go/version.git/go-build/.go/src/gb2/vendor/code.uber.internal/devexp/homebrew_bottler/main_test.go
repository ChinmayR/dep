package main

import (
	"testing"
)

func TestTarRegexp(t *testing.T) {
	for _, c := range []struct {
		given, release string
	}{
		{given: "cerberus-v0.1.4.el_capitan.bottle.tar.gz", release: "el_capitan"},
		{given: "cerberus-v0.1.4-4-g377a02b.el_capitan.bottle.tar.gz", release: "el_capitan"},
	} {
		_, release, err := extractPath(c.given)
		if err != nil {
			t.Logf("test got error: %s", err.Error())
			t.FailNow()
		}
		if c.release != release {
			t.Logf("got %s, expected %s", c.release, release)
			t.FailNow()
		}
	}
}
