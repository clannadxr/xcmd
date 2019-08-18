package util

import (
	"bytes"
	"io"
	"sync"
)

type Buffer struct {
	mu sync.RWMutex
	bytes.Buffer
}

func (b *Buffer) Bytes() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Buffer.Bytes()
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.ReadFrom(r)
}
