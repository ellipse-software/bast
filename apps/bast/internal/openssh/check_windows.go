//go:build windows

package openssh

import "fmt"

func missingToolError(name string) error {
	return fmt.Errorf("required OpenSSH tool %q is not available on PATH; install OpenSSH Client from Windows Optional Features", name)
}
