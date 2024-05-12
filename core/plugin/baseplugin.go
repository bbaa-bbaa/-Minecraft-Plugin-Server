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

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/fatih/color"
)

type BasePlugin struct {
	pm             pluginabi.PluginManager
	p              pluginabi.Plugin
	teleportCore   *TeleportCore
	playerInfo     *PlayerInfo
	simpleCommand  *SimpleCommand
	scoreboardCore *ScoreboardCore
	tellrawManager *TellrawManager
}

func (bp *BasePlugin) Println(a ...any) (int, error) {
	return bp.pm.Println(color.BlueString(bp.p.DisplayName()), a...)
}

func (bp *BasePlugin) Teleport(src string, dst any) error {
	return bp.teleportCore.Teleport(src, dst)
}

func (bp *BasePlugin) initCorePlugin(pm pluginabi.PluginManager) {
	pi := pm.GetPlugin("PlayerInfo")
	if pi != nil {
		bp.playerInfo = pi.(*PlayerInfo)
	}

	sc := pm.GetPlugin("ScoreboardCore")
	if sc != nil {
		bp.scoreboardCore = sc.(*ScoreboardCore)
	}

	tm := pm.GetPlugin("TellrawManager")
	if tm != nil {
		bp.tellrawManager = tm.(*TellrawManager)
	}

	tc := pm.GetPlugin("TeleportCore")
	if tc != nil {
		bp.teleportCore = tc.(*TeleportCore)
	}

	sp := pm.GetPlugin("SimpleCommand")
	if sp != nil {
		bp.simpleCommand = sp.(*SimpleCommand)
	}
}

func (bp *BasePlugin) Init(pm pluginabi.PluginManager, plugin pluginabi.Plugin) error {
	bp.pm = pm
	bp.p = plugin
	bp.initCorePlugin(pm)
	return nil
}

func (bp *BasePlugin) EnsureScoreboard(name string, criterion string, displayName []tellraw.Message) {
	if bp.scoreboardCore == nil {
		return
	}
	dName := ""
	if len(displayName) == 0 {
		dName = fmt.Sprintf(`"%s"`, name)
	} else {
		bName, _ := json.Marshal(displayName)
		dName = string(bName)
	}
	bp.scoreboardCore.ensureScoreboard(bp.p, name, criterion, dName)
}

func (bp *BasePlugin) RegisterTrigger(trigger MinecraftTrigger) (name string) {
	if bp.scoreboardCore == nil {
		return
	}
	return bp.scoreboardCore.registerTrigger(bp.p, trigger)[0]
}

func (bp *BasePlugin) RegisterTriggerBatch(trigger ...MinecraftTrigger) (name []string) {
	if bp.scoreboardCore == nil {
		return
	}
	return bp.scoreboardCore.registerTrigger(bp.p, trigger...)
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

func (bp *BasePlugin) Tellraw(Target string, msg []tellraw.Message) {
	bp.tellrawManager.Tellraw(bp.p, Target, msg)
}

func (bp *BasePlugin) TellrawError(Target string, err error) {
	if err == nil {
		return
	}
	bp.Tellraw("@a", []tellraw.Message{{Text: "内部错误", Color: tellraw.Red}, {Text: err.Error(), Color: tellraw.Yellow}})
}

func (bp *BasePlugin) GetWorldName(namespace_id string) string {
	if name, ok := worldName[namespace_id]; ok {
		return name
	}
	return namespace_id
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
