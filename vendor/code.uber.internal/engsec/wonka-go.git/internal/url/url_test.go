package url

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBase32(t *testing.T) {
	b := make([]byte, 256)

	found := false
	for i := 0; i < 100; i++ {
		_, err := rand.Read(b)
		require.NoError(t, err)

		ret := Base32WithoutPadding(b)
		require.NotEmpty(t, ret)
		if res := base32.HexEncoding.EncodeToString(b); strings.Contains(res, "=") {
			found = true
			require.NotContains(t, ret, "=")
		}
	}
	require.True(t, found, "no b32 encodings contained padding")
}
