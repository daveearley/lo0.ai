//go:build !unix && !windows

package migrate

import (
	"errors"
	"os"
)

func lockFile(*os.File) error { return errors.ErrUnsupported }

func unlockFile(*os.File) error { return errors.ErrUnsupported }
