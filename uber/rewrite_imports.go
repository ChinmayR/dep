package uber

import "strings"

// deduction for github.com/Sirupsen/logrus should be treated as deduction for github.com/sirupsen/logrus
func RewriteSirupsenImports(path string) string {
	if strings.HasPrefix(path, "github.com/Sirupsen") {
		return "github.com/sirupsen" + strings.TrimPrefix(path, "github.com/Sirupsen")
	}
	return path
}
