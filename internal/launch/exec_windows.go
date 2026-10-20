//go:build windows

package launch

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

func replaceProcess(path string, args []string, dir string) error {
	cmd := exec.Command(path, args[1:]...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
