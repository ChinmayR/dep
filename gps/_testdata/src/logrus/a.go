package logrus

import (
	"sort"

	"github.com/Sirupsen/logrus"
	"github.com/golang/dep/gps"
)

var (
	_ = sort.Strings
	_ = gps.Solve
	_ = logrus.ErrorKey
)
