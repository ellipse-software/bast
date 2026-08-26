//go:build windows

package vercel

import (
	"context"
	"io"
)

func watchResize(ctx context.Context, writeMessage func(int, []byte) error, out io.Writer) {
	_ = ctx
	_ = writeMessage
	_ = out
}
