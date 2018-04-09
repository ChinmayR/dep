package uber

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/xid"
)

const UBER_PREFIX = "[UBER]  "
const DEP_PREFIX = "[INFO]  "
const DEBUG_PREFIX = "[DEBUG] "
const CACHE_PREFIX = "[CACHE] "

var RunId string
var LogPath string
var UberLogger *log.Logger
var DebugLogger *log.Logger
var CacheLogger *log.Logger
var LogFile *os.File

func init() {
	RunId = xid.New().String()
	LogPath = getLogStoragePath(RunId)
	err := os.MkdirAll(filepath.Dir(LogPath), 0777)
	if err != nil {
		log.Fatal(err)
	}
	LogFile, err = os.OpenFile(LogPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		log.Fatal(err)
	}
	UberLogger = log.New(io.MultiWriter(os.Stdout, LogFile), UBER_PREFIX, 0)
	DebugLogger = log.New(LogFile, DEBUG_PREFIX, log.Lmicroseconds|log.LUTC)
	CacheLogger = log.New(LogFile, CACHE_PREFIX, log.Lmicroseconds|log.LUTC)
}

func getLogStoragePath(runId string) string {
	logFileName := fmt.Sprintf("%s-%s.log", time.Now().Format("2006/01/02/log-150405"), runId)
	return filepath.Join(GetCacheDir(), "logs", logFileName)
}
