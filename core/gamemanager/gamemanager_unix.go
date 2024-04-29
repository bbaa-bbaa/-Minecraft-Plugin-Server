//go:build unix

package main

import (
	"io"
	"syscall"
)

var MinecraftProcess_SysProcAttr = &syscall.SysProcAttr{
	Setpgid: true,
}

func (pty *MinecraftPty) readerWrapper(r io.Reader) io.Reader {
	return r
}

func (pty *MinecraftPty) writerWrapper(w io.WriteCloser) io.WriteCloser {
	return w
}
