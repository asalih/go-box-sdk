package utils

import (
	"fmt"
	"io"
)

// ChunkIterator yields fixed-size chunks from a stream until the expected file
// size has been consumed. The final chunk holds any remainder. It mirrors the
// source iterateChunks generator.
type ChunkIterator struct {
	r         io.Reader
	chunkSize int64
	fileSize  int64
	emitted   int64
}

// IterateChunks returns an iterator that splits r into chunkSize-sized pieces,
// reading exactly fileSize bytes in total.
func IterateChunks(r io.Reader, chunkSize, fileSize int64) *ChunkIterator {
	return &ChunkIterator{r: r, chunkSize: chunkSize, fileSize: fileSize}
}

// Next returns the next chunk. The boolean is false when iteration is complete.
// An error is returned if the stream ends before fileSize bytes are read.
func (it *ChunkIterator) Next() ([]byte, bool, error) {
	if it.emitted >= it.fileSize {
		return nil, false, nil
	}
	remaining := it.fileSize - it.emitted
	size := it.chunkSize
	if remaining < size {
		size = remaining
	}
	buf := make([]byte, size)
	n, err := io.ReadFull(it.r, buf)
	it.emitted += int64(n)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, false, fmt.Errorf("utils: stream size %d does not match expected file size %d", it.emitted, it.fileSize)
		}
		return nil, false, fmt.Errorf("utils: failed to read chunk: %w", err)
	}
	return buf, true, nil
}
