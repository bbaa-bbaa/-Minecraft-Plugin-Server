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
	"encoding/json"
	"fmt"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"github.com/fatih/color"
)

type BasePlugin struct {
	pm            pluginabi.PluginManager
	p             pluginabi.Plugin
	teleportCore  *TeleportCore
	playerInfo    *PlayerInfo
	simpleCommand *SimpleCommand
}

func (bp *BasePlugin) Println(a ...any) (int, error) {
	return bp.pm.Println(color.BlueString(bp.p.Name()), a...)
}

func (bp *BasePlugin) Teleport(src string, dst any) error {
	return bp.teleportCore.Teleport(src, dst)
}

func (bp *BasePlugin) Init(pm pluginabi.PluginManager, plugin pluginabi.Plugin) error {
	bp.pm = pm
	bp.p = plugin
	ignoreErr := false
	switch plugin.(type) {
	case *TeleportCore, *PlayerInfo, *SimpleCommand:
		ignoreErr = true
	}
	pi := pm.GetPlugin("PlayerInfo")
	if pi == nil {
		if !ignoreErr {
			return fmt.Errorf("no playerinfo instance")
		}
	} else {
		bp.playerInfo = pi.(*PlayerInfo)
	}

	tc := pm.GetPlugin("TeleportCore")
	if tc == nil {
		if !ignoreErr {
			return fmt.Errorf("no Teleport Core instance")
		}
	} else {
		bp.teleportCore = tc.(*TeleportCore)
	}

	sp := pm.GetPlugin("SimpleCommand")
	if sp == nil {
		if !ignoreErr {
			return fmt.Errorf("no simplecommand instance")
		}
	} else {
		bp.simpleCommand = sp.(*SimpleCommand)
	}

	return nil
}

func (bp *BasePlugin) RegisterCommand(command string, commandFunc func(string, ...string)) error {
	if bp.simpleCommand == nil {
		return fmt.Errorf("no simplecommand instance")
	}
	return bp.simpleCommand.RegisterCommand(bp.p, command, commandFunc)
}

func (bp *BasePlugin) GetPlayerInfoCache(player string) (*MinecraftPlayerInfo, error) {
	if bp.playerInfo == nil {
		return nil, fmt.Errorf("no playerInfo instance")
	}
	return bp.playerInfo.GetPlayerInfo(player, false)
}

func (bp *BasePlugin) GetPlayerInfo(player string) (*MinecraftPlayerInfo, error) {
	if bp.playerInfo == nil {
		return nil, fmt.Errorf("no playerInfo instance")
	}
	return bp.playerInfo.GetPlayerInfo(player, true)
}

func (bp *BasePlugin) GetPlayerList() []string {
	if bp.playerInfo == nil {
		return nil
	}
	return bp.playerInfo.playerList
}

func (bp *BasePlugin) RunCommand(command string) string {
	return bp.pm.RunCommand(command)
}

func (bp *BasePlugin) Tellraw(Target string, msg []TellrawMessage) string {
	msg = append([]TellrawMessage{
		{Text: "[", Color: "yellow", Bold: true},
		{Text: bp.p.DisplayName(), Color: "green", Bold: true},
		{Text: "] ", Color: "yellow", Bold: true},
	}, msg...)
	jsonMsg, _ := json.Marshal(msg)
	return bp.pm.RunCommand(fmt.Sprintf("tellraw %s %s", Target, jsonMsg))
}

func (bp *BasePlugin) Name() string {
	return "BasePlugin"
}

func (bp *BasePlugin) DisplayName() string {
	return "基础插件"
}

func (bp *BasePlugin) Pause() {

}

func (bp *BasePlugin) Start() {

}
