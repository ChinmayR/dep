package url

import (
	"encoding/base32"
	"strings"
)

// Base32WithoutPadding returns the provided bytes in base32 hex
// encoding sans any trailing padding.
func Base32WithoutPadding(b []byte) string {
	res := base32.HexEncoding.EncodeToString(b)
	return strings.Replace(res, "=", "", -1)
}
