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

package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"github.com/fatih/color"
)

type SimpleCommand struct {
	BasePlugin
	playerCommand    *regexp.Regexp
	registerCommands map[string]func(string, ...string)
	lock             sync.RWMutex
}

func (sp *SimpleCommand) Init(pm pluginabi.PluginManager) (err error) {
	err = sp.BasePlugin.Init(pm, sp)
	if err != nil {
		return err
	}
	pm.RegisterLogProcesser(sp, sp.processCommand)
	sp.playerCommand = regexp.MustCompile(`.*?\]:(?: \[[^\]]+\])? <(.*?)>.*?!!(.*)`)
	sp.registerCommands = make(map[string]func(string, ...string))
	return nil
}

func (sp *SimpleCommand) RegisterCommand(context pluginabi.PluginName, command string, commandFunc func(string, ...string)) error {
	sp.lock.Lock()
	defer sp.lock.Unlock()
	if _, ok := sp.registerCommands[command]; !ok {
		sp.Println(color.YellowString("插件 "), color.BlueString(context.DisplayName()), color.YellowString(" 注册了一条新命令: "), color.GreenString(command))
		sp.registerCommands[command] = commandFunc
	} else {
		sp.Println(color.YellowString("插件 "), color.BlueString(context.DisplayName()), color.RedString(" 尝试注册已注册的命令: "), color.GreenString(command))
		return fmt.Errorf("command exist")
	}
	return nil
}

func (sp *SimpleCommand) processCommand(logText string) {
	cmdInfo := sp.playerCommand.FindStringSubmatch(logText)
	if len(cmdInfo) < 3 {
		return
	}
	player := strings.TrimSpace(cmdInfo[1])
	rawCommand := strings.TrimSpace(cmdInfo[2])
	commandPart := strings.Split(rawCommand, " ")
	command := commandPart[0]
	sp.lock.RLock()
	if commandFunc, ok := sp.registerCommands[command]; ok {
		sp.lock.RUnlock()
		go commandFunc(player, commandPart[1:]...)
	} else {
		sp.lock.RUnlock()
	}
}

func (sp *SimpleCommand) Name() string {
	return "SimpleCommand"
}

func (sp *SimpleCommand) DisplayName() string {
	return "简单命令"
}
