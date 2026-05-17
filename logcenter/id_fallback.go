package logcenter

import (
	"strconv"
	"sync/atomic"
	"time"
)

var fallbackCounter uint64

func fallbackID() string {
	value := atomic.AddUint64(&fallbackCounter, 1)
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "_" + strconv.FormatUint(value, 36)
}
