//go:build windows

package migrate

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// The whole file is locked: LockFileEx locks a byte range, and any range
// works as long as every lo0 asks for the same one.
const lockBytes = 1

func lockFile(f *os.File) error {
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, lockBytes, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrBusy
	}
	return err
}

func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockBytes, 0, &ol)
}
