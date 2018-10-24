package logrus

import (
	"math/rand"
	"strconv"

	"github.com/Sirupsen/logrus"
)

var (
	_ = rand.Int()
	_ = strconv.Unquote
	_ = logrus.ErrorKey
)
