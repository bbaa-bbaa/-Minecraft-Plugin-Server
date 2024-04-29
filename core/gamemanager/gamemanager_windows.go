//go:build windows

package main

import (
	"io"
	"syscall"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var MinecraftProcess_SysProcAttr = &syscall.SysProcAttr{
	HideWindow: true,
}

func (pty *MinecraftPty) readerWrapper(r io.Reader) io.Reader {
	return transform.NewReader(r, simplifiedchinese.GBK.NewDecoder())
}

func (pty *MinecraftPty) writerWrapper(w io.WriteCloser) io.WriteCloser {
	return transform.NewWriter(w, simplifiedchinese.GBK.NewEncoder())
}
