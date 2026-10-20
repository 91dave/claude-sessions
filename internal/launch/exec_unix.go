//go:build !windows

package launch

import (
	"os"
	"syscall"
)

func replaceProcess(path string, args []string, dir string) error {
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return err
		}
	}
	return syscall.Exec(path, args, os.Environ())
}
