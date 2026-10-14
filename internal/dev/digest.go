package dev

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

// File metadata is not a content version: even ctime can remain equal across
// rapid writes with restored mtime. Each freshness check reads current bytes.
// A scan reuses only its scratch space, never file contents or digests.
type digestScan struct {
	hash   hash.Hash
	buffer []byte
}

func (s *digestScan) sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if s.hash == nil {
		s.hash = sha256.New()
		s.buffer = make([]byte, 32*1024)
	}
	s.hash.Reset()
	// Hide File.WriteTo so CopyBuffer uses this scan's scratch buffer.
	if _, err := io.CopyBuffer(s.hash, struct{ io.Reader }{f}, s.buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(s.hash.Sum(nil)), nil
}
