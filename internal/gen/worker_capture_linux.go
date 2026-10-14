package gen

import (
	"os"
	"syscall"
	"time"
)

func captureChangeTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
	}
	return time.Time{}
}
