package dev

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"time"
)

// Only kernel change metadata permits reusing a content digest. In particular,
// size and mtime alone miss equal-size writes followed by Chtimes. Platforms
// without a change stamp reread bytes. Entries live for one source selection.
type fileDigests struct {
	mu      sync.Mutex
	entries map[string]fileDigest
	reads   uint64
}

type fileDigest struct {
	info    os.FileInfo
	changed time.Time
	sum     string
}

type digestScan struct {
	cache *fileDigests
	next  map[string]fileDigest
}

func (c *fileDigests) scan() *digestScan {
	if c == nil {
		return &digestScan{}
	}
	c.mu.Lock()
	return &digestScan{cache: c, next: make(map[string]fileDigest, len(c.entries))}
}

func (s *digestScan) close() {
	if s.cache != nil {
		s.cache.entries = s.next
		s.cache.mu.Unlock()
	}
}

func sameVersion(a os.FileInfo, ac time.Time, b os.FileInfo, bc time.Time) bool {
	return !ac.IsZero() && ac == bc && a.Size() == b.Size() && a.ModTime() == b.ModTime() && os.SameFile(a, b)
}

func (s *digestScan) sum(path string) (string, error) {
	if s.cache == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	changed := changeTime(info)
	entry, ok := s.next[path]
	if !ok {
		entry, ok = s.cache.entries[path]
	}
	if ok && sameVersion(entry.info, entry.changed, info, changed) {
		s.next[path] = entry
		return entry.sum, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s.cache.reads++
	hash := sha256.Sum256(data)
	sum := hex.EncodeToString(hash[:])
	after, err := os.Stat(path)
	if err == nil && sameVersion(info, changed, after, changeTime(after)) {
		s.next[path] = fileDigest{info: info, changed: changed, sum: sum}
	}
	return sum, nil
}
