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
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/tellraw"
	"github.com/fatih/color"
)

type BasePlugin struct {
	pm             pluginabi.PluginManager
	p              pluginabi.Plugin
	teleportCore   *TeleportCore
	playerInfo     *PlayerInfo
	simpleCommand  *SimpleCommand
	scoreboardCore *ScoreboardCore
}

func (bp *BasePlugin) Println(a ...any) (int, error) {
	return bp.pm.Println(color.BlueString(bp.p.DisplayName()), a...)
}

func (bp *BasePlugin) Teleport(src string, dst any) error {
	return bp.teleportCore.Teleport(src, dst)
}

func (bp *BasePlugin) Init(pm pluginabi.PluginManager, plugin pluginabi.Plugin) error {
	bp.pm = pm
	bp.p = plugin
	ignoreErr := false
	switch plugin.(type) {
	case *TeleportCore, *PlayerInfo, *SimpleCommand, *ScoreboardCore:
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

	sc := pm.GetPlugin("ScoreboardCore")
	if sc == nil {
		if !ignoreErr {
			return fmt.Errorf("no scoreboard instance")
		}
	} else {
		bp.scoreboardCore = sc.(*ScoreboardCore)
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

func (bp *BasePlugin) EnsureScoreboard(name string, criterion string, displayname ...string) {
	if bp.scoreboardCore == nil {
		return
	}
	dName := ""
	if len(displayname) == 0 {
		dName = name
	} else {
		dName = displayname[0]
	}
	bp.scoreboardCore.ensureScoreboard(bp.p, name, criterion, dName)
}

func (bp *BasePlugin) DisplayScoreboard(name string, slot string) {
	if bp.scoreboardCore == nil {
		return
	}
	bp.scoreboardCore.displayScoreboard(bp.p, name, slot)
}

func (bp *BasePlugin) ScoreAction(player string, name string, action string, count int64) {
	if bp.scoreboardCore == nil {
		return
	}
	bp.scoreboardCore.scoreAction(bp.p, player, name, action, count)
}

func (bp *BasePlugin) GetAllScore() (scores map[string]map[string]int64) {
	if bp.scoreboardCore == nil {
		return
	}
	return bp.scoreboardCore.getAllScore()
}

func (bp *BasePlugin) GetOneScore(player string, name string) (scores int64) {
	if bp.scoreboardCore == nil {
		return
	}
	return bp.scoreboardCore.getOneScore(bp.p, player, name)
}

func (bp *BasePlugin) RegisterCommand(command string, commandFunc func(string, ...string)) error {
	if bp.simpleCommand == nil {
		return fmt.Errorf("no simplecommand instance")
	}
	return bp.simpleCommand.RegisterCommand(bp.p, command, commandFunc)
}

func (bp *BasePlugin) GetPlayerInfo_Position(player string) (*MinecraftPlayerInfo, error) {
	if bp.playerInfo == nil {
		return nil, fmt.Errorf("no playerInfo instance")
	}
	return bp.playerInfo.GetPlayerInfo_Position(player)
}
func (bp *BasePlugin) GetPlayerInfo(player string) (*MinecraftPlayerInfo, error) {
	if bp.playerInfo == nil {
		return nil, fmt.Errorf("no playerInfo instance")
	}
	return bp.playerInfo.GetPlayerInfo(player)
}

func (bp *BasePlugin) GetPlayerList() []string {
	if bp.playerInfo == nil {
		return nil
	}
	return bp.playerInfo.GetPlayerList()
}

func (bp *BasePlugin) RunCommand(command string) string {
	return bp.pm.RunCommand(command)
}

func (bp *BasePlugin) Tellraw(Target string, msg []tellraw.Message) string {
	msg = append([]tellraw.Message{
		{Text: "[", Color: tellraw.Yellow, Bold: true},
		{Text: bp.p.DisplayName(), Color: tellraw.Green, Bold: true},
		{Text: "] ", Color: tellraw.Yellow, Bold: true},
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
