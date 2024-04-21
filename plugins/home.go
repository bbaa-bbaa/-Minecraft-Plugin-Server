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
	"strings"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
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

func (hp *HomePlugin) Init(pm pluginabi.PluginManager) error {
	hp.BasePlugin.Init(pm, hp)
	hp.RegisterCommand("home", hp.home)
	hp.RegisterCommand("sethome", hp.sethome)
	return nil
}

func (hp *HomePlugin) home(player string, args ...string) {
	if len(args) > 1 {
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "指定过多目标", Color: "red"}})
		return
	}
	home := ""
	if len(args) == 0 {
		home = "default"
	} else {
		home = args[0]
	}
	pi, err := hp.GetPlayerInfoCache(player)
	if err != nil {
		fmt.Println(pi)
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "内部错误：无法获取玩家数据", Color: "red"}})
		return
	}
	var homeList HomePlugin_HomeList
	homeListData := pi.GetExtra(hp, &homeList)
	if homeListData == nil {
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "你没有设置任何家", Color: "red"}})
		return
	} else {
		homeList = *homeListData.(*HomePlugin_HomeList)
	}
	homeNameList := maps.Keys(homeList)
	homeNameList = lo.Filter(homeNameList, func(item string, index int) bool {
		return len(item) >= len(home) && strings.EqualFold(home, item[:len(home)])
	})
	if len(homeNameList) == 0 {
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "找不到目标", Color: "red"}})
		return
	}
	homeName := homeNameList[0]
	homePosition := homeList[homeName]
	hp.Tellraw(player, []plugin.TellrawMessage{
		{Text: "2秒后TP至家 ", Color: "green", Bold: true},
		{Text: "「" + homeName + "」", Color: "aqua", HoverEvent: &plugin.TellrawMessage_HoverEvent{
			Action: "show_text", Contents: []plugin.TellrawMessage{
				{Text: "世界: ", Color: "green"},
				{Text: homePosition.Dimension, Color: "yellow"},
				{Text: "\n坐标: [", Color: "green"},
				{Text: fmt.Sprintf("%f", homePosition.Position[0]), Color: "aqua"},
				{Text: ",", Color: "yellow"},
				{Text: fmt.Sprintf("%f", homePosition.Position[1]), Color: "aqua"},
				{Text: ",", Color: "yellow"},
				{Text: fmt.Sprintf("%f", homePosition.Position[2]), Color: "aqua"},
				{Text: "]", Color: "green"},
			},
		},
		},
	})
	time.Sleep(1500 * time.Millisecond)
	hp.Teleport(player, homePosition)
}

func (hp *HomePlugin) sethome(player string, args ...string) {
	if len(args) > 1 {
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "非法的家名称", Color: "red"}})
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
		fmt.Println(err)
		hp.Tellraw(player, []plugin.TellrawMessage{{Text: "内部错误：无法获取玩家数据", Color: "red"}})
		return
	}
	var homeList HomePlugin_HomeList
	homeListData := pi.GetExtra(hp, &homeList)
	if homeListData == nil {
		homeList = make(HomePlugin_HomeList)
		pi.PutExtra(hp, &homeList)
	} else {
		homeList = *homeListData.(*HomePlugin_HomeList)
	}
	homeList[home] = pi.Location
	hp.Tellraw(player, []plugin.TellrawMessage{
		{Text: "设置家 ", Color: "green", Bold: true},
		{Text: "「" + home + "」", Color: "aqua", HoverEvent: &plugin.TellrawMessage_HoverEvent{
			Action: "show_text", Contents: []plugin.TellrawMessage{
				{Text: "世界: ", Color: "green"},
				{Text: pi.Location.Dimension, Color: "yellow"},
				{Text: "\n坐标: [", Color: "green"},
				{Text: fmt.Sprintf("%f", pi.Location.Position[0]), Color: "aqua"},
				{Text: ",", Color: "yellow"},
				{Text: fmt.Sprintf("%f", pi.Location.Position[1]), Color: "aqua"},
				{Text: ",", Color: "yellow"},
				{Text: fmt.Sprintf("%f", pi.Location.Position[2]), Color: "aqua"},
				{Text: "]", Color: "green"},
			},
		},
		},
	})
	pi.Commit()
}
