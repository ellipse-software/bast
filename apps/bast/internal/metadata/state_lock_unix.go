//go:build !windows

package metadata

import (
	"os"
	"syscall"
)

func lockMetadataFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockMetadataFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
