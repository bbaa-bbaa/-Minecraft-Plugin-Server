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
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/tellraw"
	"github.com/fatih/color"
)

var DeathEventLog = regexp.MustCompile(`MinecraftServer.*\]: (\w+) [\w ]+.*?`)

var DeathEventBlackList = []*regexp.Regexp{regexp.MustCompile("has made the advancement")}

type BackPlugin struct {
	plugin.BasePlugin
}

func (bp *BackPlugin) DisplayName() string {
	return "返回上一地点"
}

func (bp *BackPlugin) Name() string {
	return "BackPlugin"
}

func (bp *BackPlugin) back(player string, _ ...string) {
	pi, err := bp.GetPlayerInfo_Position(player)
	if err != nil {
		bp.Tellraw(player, []tellraw.Message{{Text: "内部错误：无法获取玩家数据", Color: tellraw.Red}})
		return
	}
	bp.Tellraw(player, []tellraw.Message{
		{Text: "2秒后TP至 ", Color: tellraw.Green, Bold: true},
		{Text: "「上一地点」", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: "show_text", Contents: []tellraw.Message{
					{Text: "世界: ", Color: tellraw.Green},
					{Text: pi.Location.Dimension, Color: tellraw.Yellow},
					{Text: "\n坐标: [", Color: tellraw.Green},
					{Text: fmt.Sprintf("%f", pi.Location.Position[0]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", pi.Location.Position[1]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", pi.Location.Position[2]), Color: tellraw.Aqua},
					{Text: "]", Color: tellraw.Green},
				},
			},
		},
	})
	time.Sleep(1500 * time.Millisecond)
	bp.Teleport(player, pi.LastLocation)
}

func (bp *BackPlugin) deathEvent(logmsg string, iscmdrsp bool) {
	if iscmdrsp {
		return
	}
	if DeathEventLog.MatchString(logmsg) {
		for _, black := range DeathEventBlackList {
			if black.MatchString(logmsg) {
				return
			}
		}
		logmsg := DeathEventLog.FindStringSubmatch(logmsg)
		player := logmsg[1]
		playerList := bp.GetPlayerList()
		if slices.Contains(playerList, player) {
			playerDeathData := bp.RunCommand(fmt.Sprintf("data get entity %s DeathTime", player))
			playerDeathTime := strings.Split(playerDeathData, ":")
			if len(playerDeathTime) != 2 {
				return
			}
			deathTime, err := strconv.ParseInt(strings.Trim(playerDeathTime[1], " s"), 10, 64)
			if err != nil {
				fmt.Println(err)
				return
			}
			if deathTime > 0 {
				bp.Println(color.GreenString(player), color.YellowString(" 不幸离世，保存死亡地点"))
				pi, err := bp.GetPlayerInfo_Position(player)
				if err != nil {
					fmt.Println(err)
					return
				}
				pi.LastLocation = pi.Location
				pi.Commit()
			}
		}
	}
}

func (bp *BackPlugin) Init(pm pluginabi.PluginManager) (err error) {
	err = bp.BasePlugin.Init(pm, bp)
	if err != nil {
		return err
	}
	bp.EnsureScoreboard("Death", "deathCount", "死亡次数")
	pm.RegisterLogProcesser(bp, bp.deathEvent)
	bp.RegisterCommand("back", bp.back)
	return nil
}
