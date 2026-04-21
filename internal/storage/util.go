package storage

import "os"

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
