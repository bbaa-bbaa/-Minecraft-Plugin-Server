// Copyright 2024 bbaa
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"fmt"
	"io"
	"os"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"golang.org/x/term"
)

type REPLPlugin struct {
	pm       *MinecraftPluginManager
	terminal *term.Terminal
	state    *term.State
}

func (rp *REPLPlugin) DisplayName() string {
	return "终端命令"
}

func (rp *REPLPlugin) Name() string {
	return "REPLPlugin"
}

func (rp *REPLPlugin) RunCommand(cmd string) string {
	return rp.pm.RunCommand(cmd)
}

func (rp *REPLPlugin) initTerminal() (t *term.Terminal, err error) {
	rp.state, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	terminal := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	t = term.NewTerminal(terminal, "Minecraft-Command > ")
	return t, nil
}

func (rp *REPLPlugin) Init(pm pluginabi.PluginManager) (err error) {
	ok := false
	rp.pm, ok = pm.(*MinecraftPluginManager)
	if !ok {
		return fmt.Errorf("can not get terminal")
	}
	rp.terminal, _ = rp.initTerminal()
	go rp.worker()
	return nil
}

func (rp *REPLPlugin) worker() {
	for {
		line, err := rp.terminal.ReadLine()
		if err != nil {
			if err == io.EOF {
				os.Exit(0)
			}
		}
		if line == "exit" {
			os.Exit(0)
		}
		if len(line) > 0 {
			rp.RunCommand(line)
		}
	}
}

func (rp *REPLPlugin) Pause() {

}

func (rp *REPLPlugin) Start() {

}
