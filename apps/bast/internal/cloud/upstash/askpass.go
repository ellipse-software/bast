package upstash

import (
	"fmt"
	"io"
	"os/exec"

	"bast/internal/askpass"
)

const (
	AskPassEnv   = askpass.Env
	AskPassValue = askpass.Value
)

func IsAskPassRequest() bool {
	return askpass.IsRequest()
}

func PrintAPIKey(out io.Writer, keyFile string) error {
	key, err := ReadKeyFile(keyFile)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, key)
	return err
}

func ApplyAskPass(cmd *exec.Cmd, bastExecutable string) {
	askpass.ApplyUpstash(cmd, bastExecutable)
}
