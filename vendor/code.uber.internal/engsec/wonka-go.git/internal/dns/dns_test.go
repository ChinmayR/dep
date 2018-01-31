package dns

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"

	"github.com/stretchr/testify/assert"
)

func TestNetClient(t *testing.T) {
	c := NewNetClient()
	// TODO(tjulian): this make a real DNS request.. should we make that less fragile?
	records, err := c.LookupTXT(context.Background(), "uber.com")
	assert.NoError(t, err)
	assert.True(t, len(records) > 0, "Domain uber.com should have TXT records.")
}

func TestFSClient(t *testing.T) {
	mockRecord := "text"
	tmpfile := testFileWithContents(t, mockRecord)
	defer os.Remove(tmpfile.Name())

	dir := filepath.Dir(tmpfile.Name())
	txtPath := filepath.Base(tmpfile.Name())
	c := NewFSClient(dir)
	records, err := c.LookupTXT(context.Background(), txtPath)
	assert.NoError(t, err)
	assert.True(t, len(records) > 0, "File %q should have TXT records.", tmpfile.Name())
	assert.Equal(t, mockRecord, records[0])
}

func testFileWithContents(t *testing.T, contents string) *os.File {
	tmpfile, err := ioutil.TempFile("", "dns-fs-test")
	assert.NoError(t, err)
	_, err = tmpfile.Write([]byte(contents))
	assert.NoError(t, err)
	err = tmpfile.Close()
	assert.NoError(t, err)
	return tmpfile
}

func TestFSClientNoRecords(t *testing.T) {
	tmpfile := testFileWithContents(t, "")
	defer os.Remove(tmpfile.Name())

	dir := filepath.Dir(tmpfile.Name())
	txtPath := filepath.Base(tmpfile.Name())
	c := NewFSClient(dir)
	_, err := c.LookupTXT(context.Background(), txtPath)
	assert.Error(t, err)
}

func TestFSClientDestNotFound(t *testing.T) {
	c := NewFSClient(".")
	_, err := c.LookupTXT(context.Background(), "this-definitely-doesnt-exist")
	assert.Error(t, err)
}

func TestMockClient(t *testing.T) {
	mockRecords := []string{"foo", "bar"}
	c := NewMockClient(mockRecords)
	records, err := c.LookupTXT(context.Background(), "fake-pubkey-hash.uberinternal.com")
	assert.NoError(t, err)
	assert.True(t, len(records) == len(mockRecords), "Mock uber.com should have TXT records.")
	assert.Equal(t, mockRecords[0], records[0])
	assert.Equal(t, mockRecords[1], records[1])
}

func TestMockClientNoRecords(t *testing.T) {
	c := NewMockClient(nil)
	_, err := c.LookupTXT(context.Background(), "uber.com")
	assert.Error(t, err)
}

func TestNewClient(t *testing.T) {
	// test default net client
	c := newClient()
	_ = c.(netClient)

	// test env triggers filesystem client
	defer testhelper.SetEnvVar(WonkaPanicDirEnv, ".")()
	c = newClient()
	_ = c.(fsClient)
}
