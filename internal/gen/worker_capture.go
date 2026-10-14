package gen

import (
	"os"
	"time"
)

// A worker retains immutable bytes only while the kernel still identifies the
// same file revision. Directory membership is rediscovered by each capture.
// Size and mtime alone are insufficient: editors may preserve both on a write.
type capturedFile struct {
	info    os.FileInfo
	changed time.Time
	data    []byte
}

type captureReader struct {
	previous map[string]capturedFile
	next     map[string]capturedFile
	readFile func(string) ([]byte, error)
}

func sameCapturedVersion(a os.FileInfo, ac time.Time, b os.FileInfo, bc time.Time) bool {
	return !ac.IsZero() && ac == bc && a.Size() == b.Size() && a.ModTime() == b.ModTime() && os.SameFile(a, b)
}

func (r *captureReader) read(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	changed := captureChangeTime(info)
	if old, ok := r.previous[path]; ok && sameCapturedVersion(old.info, old.changed, info, changed) {
		r.next[path] = old
		return old.data, nil
	}
	data, err := r.readFile(path)
	if err != nil {
		return nil, err
	}
	// A write during the read cannot seed a later cache hit.
	after, err := os.Stat(path)
	if err == nil && sameCapturedVersion(info, changed, after, captureChangeTime(after)) {
		r.next[path] = capturedFile{info: info, changed: changed, data: data}
	}
	return data, nil
}
