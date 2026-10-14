//go:build !darwin && !linux

package gen

import (
	"os"
	"time"
)

func captureChangeTime(os.FileInfo) time.Time { return time.Time{} }
