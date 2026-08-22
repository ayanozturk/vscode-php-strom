//go:build !darwin && !linux

package indexer

import "os"

func openFileForBoundedRead(path string) (*os.File, error) {
	return os.Open(path)
}
