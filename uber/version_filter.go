package uber

import (
	"strings"
)

var keywordsToFilter = []string{
	"phabricator", //phabricator/base/2192296
	"revisions",   //farc/revisions/D1118353
}

func IsValidVersion(typedVersion string) bool {
	for _, keyword := range keywordsToFilter {
		if strings.Contains(typedVersion, keyword) {
			return false
		}
	}

	return true
}
