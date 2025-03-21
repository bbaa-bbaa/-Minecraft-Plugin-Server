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
	"unicode"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/fatih/color"
)

var MinecraftMessage = regexp.MustCompile(`\[.*?MinecraftServer.?\]: (.*)`)

var DeathEventBlackList = []*regexp.Regexp{regexp.MustCompile(" has the following entity data"), regexp.MustCompile("trigger"), regexp.MustCompile("players online"), core.PlayerJoinLeaveMessage}

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
	pi, err := bp.GetPlayerInfo(player)
	if err != nil {
		bp.Tellraw(player, []tellraw.Message{{Text: "内部错误：无法获取玩家数据", Color: tellraw.Red}})
		return
	}
	if pi.LastLocation == nil {
		bp.Tellraw(player, []tellraw.Message{{Text: "找不到历史地点记录", Color: tellraw.Red}})
		return
	}
	bp.Tellraw(player, []tellraw.Message{
		{Text: "2秒后TP至 ", Color: tellraw.Green, Bold: true},
		{Text: "「上一地点」", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: "show_text", Contents: []tellraw.Message{
					{Text: "世界: ", Color: tellraw.Green},
					{Text: pi.LastLocation.Dimension, Color: tellraw.Yellow},
					{Text: "\n坐标: [", Color: tellraw.Green},
					{Text: fmt.Sprintf("%f", pi.LastLocation.Position[0]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", pi.LastLocation.Position[1]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", pi.LastLocation.Position[2]), Color: tellraw.Aqua},
					{Text: "]", Color: tellraw.Green},
				},
			},
		},
	})
	time.Sleep(1500 * time.Millisecond)
	bp.Teleport(player, pi.LastLocation)
}

func (bp *BackPlugin) checkDeath(player string) {
	playerDeathData := bp.RunCommand(fmt.Sprintf("data get entity %s DeathTime", player))
	playerDeathTime := strings.Split(playerDeathData, ":")
	if len(playerDeathTime) != 2 {
		return
	}
	deathTime, err := strconv.ParseInt(strings.Trim(playerDeathTime[1], " s"), 10, 64)
	if err != nil {
		return
	}
	if deathTime > 0 {
		bp.Println(color.GreenString(player), color.YellowString(" 不幸离世，保存死亡地点"))
		pi, err := bp.GetPlayerInfo(player)
		if err != nil {
			bp.Tellraw(player, []tellraw.Message{
				{Text: "死亡地点记录失败", Color: tellraw.Green},
				{Text: " 请联系服务器管理tp", Color: tellraw.Red},
			})
			return
		}
		position, err := pi.GetDeathPosition()
		if err != nil {
			position, err = pi.GetPosition()
			if err != nil {
				bp.Tellraw(player, []tellraw.Message{
					{Text: "死亡地点记录失败", Color: tellraw.Green},
					{Text: " 请联系服务器管理tp", Color: tellraw.Red},
				})
				return
			}
		}
		if pi.LastLocation == nil || (pi.LastLocation.Dimension != position.Dimension || !slices.Equal(pi.LastLocation.Position[:], position.Position[:])) {
			pi.LastLocation = position
			pi.Commit()
			bp.Tellraw(player, []tellraw.Message{
				{Text: "已保存上次死亡地点", Color: tellraw.Green},
				{Text: " 输入 !!back 传送", Color: tellraw.Yellow},
			})
			//			bp.RunCommand("effect give @a minecraft:glowing infinite 1 true")
		}
	}
}

func (bp *BackPlugin) deathEvent(logmsg string, iscmdrsp bool) {
	if contentMatcher := MinecraftMessage.FindStringSubmatch(logmsg); len(contentMatcher) == 2 {
		for _, black := range DeathEventBlackList {
			if black.MatchString(logmsg) {
				return
			}
		}
		if iscmdrsp {
			return
		}
		playerList := bp.GetPlayerList()
		playerSelect := strings.Join(playerList, "|")
		playerRegex, _ := regexp.Compile(fmt.Sprintf(`\b(%s)\b`, playerSelect))
		playerMatcher := playerRegex.FindAllStringSubmatch(contentMatcher[1], -1)
		time.Sleep(20 * time.Millisecond)
		for _, matchPlayer := range playerMatcher {
			player := matchPlayer[1]
			matchIndex := strings.Index(logmsg, player)
			if matchIndex >= 0 {
				if !unicode.IsSpace([]rune(logmsg)[max(0, matchIndex-1)]) && !unicode.IsSpace([]rune(logmsg)[min(len(logmsg)-1, matchIndex+len(player))]) {
					continue
				}
			}
			bp.checkDeath(player)
		}
	}
}

func (bp *BackPlugin) Init(pm pluginabi.PluginManager) (err error) {
	err = bp.BasePlugin.Init(pm, bp)
	if err != nil {
		return err
	}
	bp.EnsureScoreboard("Death", "deathCount", []tellraw.Message{
		{Text: "重", Color: tellraw.Red, Bold: true},
		{Text: "开", Color: tellraw.Light_Purple},
		{Text: "★", Color: tellraw.Aqua, Bold: true},
		{Text: "次", Color: tellraw.Green, Bold: true},
		{Text: "数", Color: tellraw.Yellow},
	})
	bp.DisplayScoreboard("Death", "sidebar")
	bp.RegisterLogProcesser(bp.deathEvent)
	bp.RegisterCommand("back", bp.back)
	return nil
}
