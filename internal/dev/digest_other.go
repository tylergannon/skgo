//go:build !darwin && !linux

package dev

import (
	"os"
	"time"
)

func changeTime(os.FileInfo) time.Time { return time.Time{} }
