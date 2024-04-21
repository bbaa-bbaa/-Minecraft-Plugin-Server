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

package plugins

import (
	"strings"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/tellraw"
	"github.com/samber/lo"
)

type TeleportPlugin struct {
	plugin.BasePlugin
}

func (tp *TeleportPlugin) DisplayName() string {
	return "传送命令"
}

func (tp *TeleportPlugin) Name() string {
	return "TeleportPlugin"
}

func (tp *TeleportPlugin) Init(pm pluginabi.PluginManager) error {
	tp.BasePlugin.Init(pm, tp)
	tp.RegisterCommand("tp", tp.teleport)
	return nil
}

func (tp *TeleportPlugin) teleport(player string, arg ...string) {
	if len(arg) != 1 {
		tp.Tellraw(player, []tellraw.Message{{Text: "未指定或指定过多目标", Color: tellraw.Red}})
		return
	}
	targetName := arg[0]
	playerList := tp.GetPlayerList()
	playerList = lo.Filter(playerList, func(item string, index int) bool {
		return len(item) >= len(targetName) && strings.EqualFold(targetName, item[:len(targetName)])
	})
	if len(playerList) != 1 {
		if len(playerList) == 0 {
			tp.Tellraw(player, []tellraw.Message{{Text: "找不到目标", Color: tellraw.Red}})
		} else {
			tp.Tellraw(player, []tellraw.Message{{Text: "非唯一目标", Color: tellraw.Red}})
		}
		return
	}
	go func() {
		tp.Tellraw(playerList[0], []tellraw.Message{{Text: "2秒后 ", Color: tellraw.Green, Bold: true}, {Type: tellraw.Selector, Selector: player, Color: tellraw.Yellow}, {Text: " TP至你", Color: tellraw.Green, Bold: true}})
		tp.Tellraw(player, []tellraw.Message{{Text: "2秒后TP至 ", Color: tellraw.Green, Bold: true}, {Type: tellraw.Selector, Selector: playerList[0], Color: tellraw.Yellow, Bold: true}})
	}()
	time.Sleep(1500 * time.Millisecond)
	err := tp.Teleport(player, playerList[0])
	if err != nil {
		tp.Tellraw(player, []tellraw.Message{{Text: err.Error(), Color: tellraw.Red}})
	}
}
