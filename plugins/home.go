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
	"slices"
	"strings"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
)

type HomePlugin_HomeList map[string]*plugin.MinecraftPosition

type HomePlugin struct {
	plugin.BasePlugin
}

func (hp *HomePlugin) DisplayName() string {
	return "家"
}

func (hp *HomePlugin) Name() string {
	return "HomePlugin"
}

func (hp *HomePlugin) Init(pm pluginabi.PluginManager) (err error) {
	err = hp.BasePlugin.Init(pm, hp)
	if err != nil {
		return err
	}
	hp.RegisterCommand("home", hp.home)
	hp.RegisterCommand("sethome", hp.sethome)
	hp.RegisterCommand("homelist", hp.homelist)
	hp.RegisterCommand("delhome", hp.delhome)
	return nil
}

func (hp *HomePlugin) home(player string, args ...string) {
	if len(args) > 1 {
		hp.Tellraw(player, []tellraw.Message{{Text: "指定过多目标", Color: tellraw.Red}})
		return
	}
	home := ""
	if len(args) == 0 {
		home = "default"
	} else {
		home = args[0]
	}
	pi, err := hp.GetPlayerInfo(player)
	if err != nil {
		hp.TellrawError("@a", err)
		return
	}
	var homeList HomePlugin_HomeList
	pi.GetExtra(hp, &homeList)
	if homeList == nil {
		hp.Tellraw(player, []tellraw.Message{{Text: "你没有设置任何家", Color: tellraw.Red}})
		return
	}
	homeNameList := maps.Keys(homeList)
	homeNameList = lo.Filter(homeNameList, func(item string, index int) bool {
		return len(item) >= len(home) && strings.EqualFold(home, item[:len(home)])
	})
	slices.SortFunc(homeNameList, func(a string, b string) int {
		return len(a) - len(b)
	})
	if len(homeNameList) == 0 {
		hp.Tellraw(player, []tellraw.Message{{Text: "你没有设置家 ", Color: tellraw.Red}, {Text: home, Color: tellraw.Yellow}})
		return
	}
	homeName := homeNameList[0]
	homePosition := homeList[homeName]
	hp.Tellraw(player, []tellraw.Message{
		{Text: "2秒后TP至家 ", Color: tellraw.Green, Bold: true},
		{Text: "「" + homeName + "」", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: tellraw.Show_Text, Contents: []tellraw.Message{
					{Text: "世界: ", Color: tellraw.Green},
					{Text: homePosition.Dimension, Color: tellraw.Yellow},
					{Text: "\n坐标: [", Color: tellraw.Green},
					{Text: fmt.Sprintf("%f", homePosition.Position[0]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", homePosition.Position[1]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", homePosition.Position[2]), Color: tellraw.Aqua},
					{Text: "]", Color: tellraw.Green},
				},
			},
			ClickEvent: &tellraw.ClickEvent{
				Action: tellraw.SuggestCommand,
				Value:  "!!home " + home,
			},
		},
	})
	time.Sleep(1500 * time.Millisecond)
	hp.Teleport(player, homePosition)
}

func (hp *HomePlugin) sethome(player string, args ...string) {
	if len(args) > 1 {
		hp.Tellraw(player, []tellraw.Message{{Text: "非法的家名称", Color: tellraw.Red}})
		return
	}
	home := ""
	if len(args) == 0 {
		home = "default"
	} else {
		home = args[0]
	}
	pi, err := hp.GetPlayerInfo_Position(player)
	if err != nil {
		hp.TellrawError("@a", err)
		return
	}
	var homeList HomePlugin_HomeList
	pi.GetExtra(hp, &homeList)
	if homeList == nil {
		homeList = make(HomePlugin_HomeList)
		pi.PutExtra(hp, homeList)
	}
	homeList[home] = pi.Location
	hp.Tellraw(player, []tellraw.Message{
		{Text: "设置家 ", Color: tellraw.Green, Bold: true},
		{Text: "「" + home + "」", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: tellraw.Show_Text, Contents: []tellraw.Message{
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
			ClickEvent: &tellraw.ClickEvent{
				Action: tellraw.SuggestCommand,
				Value:  "!!home " + home,
			},
		},
	})
	pi.Commit()
}

func (hp *HomePlugin) homelist(player string, args ...string) {
	if len(args) > 0 {
		hp.Tellraw(player, []tellraw.Message{{Text: "指定过多参数", Color: tellraw.Red}})
		return
	}
	pi, err := hp.GetPlayerInfo(player)
	if err != nil {
		hp.TellrawError("@a", err)
		return
	}
	var homeList HomePlugin_HomeList
	pi.GetExtra(hp, &homeList)
	if homeList == nil {
		hp.Tellraw(player, []tellraw.Message{{Text: "你没有设置任何家", Color: tellraw.Red}})
		return
	}
	if len(homeList) == 0 {
		hp.Tellraw(player, []tellraw.Message{{Text: "你没有设置任何家", Color: tellraw.Red}})
		return
	}
	homeMsg := []tellraw.Message{{Text: "你拥有以下家:", Color: tellraw.Green}, {Text: "[点击可快速传送]\n", Color: tellraw.Light_Purple, Bold: true}}
	for home, position := range homeList {
		homeMsg = append(homeMsg, tellraw.Message{
			Text: "「" + home + "」 ", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: tellraw.Show_Text, Contents: []tellraw.Message{
					{Text: "世界: ", Color: tellraw.Green},
					{Text: position.Dimension, Color: tellraw.Yellow},
					{Text: "\n坐标: [", Color: tellraw.Green},
					{Text: fmt.Sprintf("%f", position.Position[0]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", position.Position[1]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", position.Position[2]), Color: tellraw.Aqua},
					{Text: "]", Color: tellraw.Green},
				},
			},
			ClickEvent: &tellraw.ClickEvent{
				Action: tellraw.RunCommand,
				GoFunc: func(tplayer string, i int) {
					hp.home(player, home)
				},
			},
		})
	}
	hp.Tellraw(player, homeMsg)
}

func (hp *HomePlugin) delhome(player string, args ...string) {
	if len(args) > 1 {
		hp.Tellraw(player, []tellraw.Message{{Text: "非法的家名称", Color: tellraw.Red}})
		return
	}
	home := ""
	if len(args) == 0 {
		home = "default"
	} else {
		home = args[0]
	}
	pi, err := hp.GetPlayerInfo(player)
	if err != nil {
		hp.TellrawError("@a", err)
		return
	}
	var homeList HomePlugin_HomeList
	pi.GetExtra(hp, &homeList)
	if homeList == nil {
		homeList = make(HomePlugin_HomeList)
		pi.PutExtra(hp, homeList)
	}
	homeInfo, ok := homeList[home]
	if !ok {
		hp.Tellraw(player, []tellraw.Message{{Text: "你没有设置家", Color: tellraw.Red, Bold: true}, {Text: "「" + home + "」", Color: tellraw.Aqua}})
		return
	}
	hp.Tellraw(player, []tellraw.Message{
		{Text: "删除家 ", Color: tellraw.Red, Bold: true},
		{Text: "「" + home + "」", Color: tellraw.Aqua,
			HoverEvent: &tellraw.HoverEvent{
				Action: tellraw.Show_Text, Contents: []tellraw.Message{
					{Text: "世界: ", Color: tellraw.Green},
					{Text: homeInfo.Dimension, Color: tellraw.Yellow},
					{Text: "\n坐标: [", Color: tellraw.Green},
					{Text: fmt.Sprintf("%f", homeInfo.Position[0]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", homeInfo.Position[1]), Color: tellraw.Aqua},
					{Text: ",", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%f", homeInfo.Position[2]), Color: tellraw.Aqua},
					{Text: "]", Color: tellraw.Green},
				},
			},
		},
	})
	delete(homeList, home)
	pi.Commit()
}
