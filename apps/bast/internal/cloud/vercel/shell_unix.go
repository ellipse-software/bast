//go:build !windows

package vercel

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

func watchResize(ctx context.Context, writeMessage func(int, []byte) error, out io.Writer) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				cols, rows := terminalSize(out)
				payload, err := EncodeResizeFrame(cols, rows)
				if err != nil {
					return
				}
				_ = writeMessage(websocket.TextMessage, payload)
			}
		}
	}()
}
