package cmd

import (
	"encoding/json"
	"net"
	"os"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serverFunc(t *testing.T, ln net.Listener, repl wonka.WonkadReply) {
	conn, err := ln.Accept()
	if err != nil {
		require.NoError(t, err, "error accepting the connection: %v", err)
	}
	defer conn.Close()

	var req wonka.WonkadRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		require.NoError(t, err, "error decoding the request: %v", err)
	}

	resp, err := json.Marshal(repl)
	require.NoError(t, err, "error marshalling response %v", err)

	_, err = conn.Write(resp)
	if err != nil {
		require.NoError(t, err, "error writing the response: %v", err)
	}
	return
}

func TestWonkadRequestErrorsWhenPathIsBad(t *testing.T) {
	req := wonka.WonkadRequest{}
	_, err := wonkadRequest("badPath", req)
	require.NotNil(t, err)
}

func TestWonkadRequestSucced(t *testing.T) {
	wonkadFile := "testUnixConnection"
	ln, err := net.Listen("unix", wonkadFile)
	require.NoError(t, err, "error setting up the listener: %v", err)
	defer ln.Close()
	defer os.Remove(wonkadFile)

	repl := wonka.WonkadReply{}
	go serverFunc(t, ln, repl)

	req := wonka.WonkadRequest{}
	resp, err := wonkadRequest(wonkadFile, req)
	require.Nil(t, err)
	assert.Equal(t, resp, repl)
}
