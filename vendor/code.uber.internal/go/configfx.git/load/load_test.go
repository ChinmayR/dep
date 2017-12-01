package load

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTestProvider(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, os.Mkdir("config", os.ModePerm))
	defer func() { assert.NoError(os.Remove("config")) }()

	base, err := os.Create(filepath.Join("config", "base.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(os.Remove(base.Name())) }()

	base.WriteString("source: base")
	base.Close()

	// Setup test.yaml
	tst, err := os.Create(filepath.Join("config", "test.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(os.Remove(tst.Name())) }()
	fmt.Fprint(tst, "dir: ${CONFIG_DIR:test}")

	p, err := TestProvider()
	require.NoError(t, err)
	assert.Equal("base", p.Get("source").String())
	assert.Equal("test", p.Get("dir").String())
}

func TestErrorWhenNoFilesLoaded(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "testConfig")
	require.NoError(t, err)
	defer func() { assert.NoError(t, os.Remove(dir)) }()

	_, err = FromFiles(nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers were loaded")
}

func TestLoadFromFiles(t *testing.T) {
	t.Parallel()

	withBase(t, func(dir string) {
		_, err := FromFiles(
			[]string{dir},
			[]FileInfo{
				{Name: "base.yaml", Interpolate: true},

				// development.yaml doesn't exist, we should skip it
				{Name: "development.yaml", Interpolate: false},
			},
			func(string) (string, bool) { return "", false },
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `default is empty for "Email"`)
	}, "${Email}")
}

func TestLoadFromFilesWithoutInterpolation(t *testing.T) {
	t.Parallel()

	withBase(t, func(dir string) {
		p, err := FromFiles(
			[]string{dir},
			[]FileInfo{{Name: "base.yaml", Interpolate: false}},
			func(string) (string, bool) { return "", false },
		)

		require.NoError(t, err)
		assert.Equal(t, "${Email}", p.Get("email").String())
	}, "email: ${Email}")
}

func withBase(t *testing.T, f func(dir string), contents string) {
	dir, err := ioutil.TempDir("", "testConfig")
	require.NoError(t, err)

	defer func() { require.NoError(t, os.Remove(dir)) }()

	base, err := os.Create(fmt.Sprintf("%s/base.yaml", dir))
	require.NoError(t, err)
	defer os.Remove(base.Name())

	base.WriteString(contents)
	base.Close()

	f(dir)
}

func TestLoadOnlyFromBase(t *testing.T) {
	t.Parallel()

	withBase(t, func(dir string) {
		p, err := FromFiles(
			[]string{dir},
			[]FileInfo{
				{Name: "base.yaml", Interpolate: true},

				// development.yaml doesn't exist, we should skip it
				{Name: "development.yaml"},
			},
			func(string) (string, bool) { return "", false },
		)

		require.NoError(t, err)
		assert.Equal(t, "value", p.Get("key").String())
	}, "key: value")
}
