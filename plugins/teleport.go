package plugins

import (
	"fmt"
	"strings"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"github.com/samber/lo"
)

type TeleportPlugin struct {
	plugin.BasePlugin
	teleportCore *plugin.TeleportCore
}

func (tp *TeleportPlugin) DisplayName() string {
	return "传送命令"
}

func (tp *TeleportPlugin) Name() string {
	return "TeleportPlugin"
}

func (tp *TeleportPlugin) Init(pm pluginabi.PluginManager) error {
	tp.BasePlugin.Init(pm, tp)
	tc := pm.GetPlugin("TeleportCore").(*plugin.TeleportCore)
	if tc == nil {
		return fmt.Errorf("无法获取 Teleport Core")
	}
	tp.teleportCore = tc
	sp := pm.GetPlugin("SimpleCommand").(*plugin.SimpleCommand)
	if sp == nil {
		return fmt.Errorf("无法注册命令 tp")
	}
	sp.RegisterCommand(tp, "tp", tp.Teleport)
	return nil
}

func (tp *TeleportPlugin) Teleport(player string, arg ...string) {
	if len(arg) != 1 {
		tp.Tellraw(player, []plugin.TellrawMessage{{Text: "未指定或指定过多目标", Color: "red"}})
		return
	}
	targetName := arg[0]
	playerList := tp.GetPlayerList()
	playerList = lo.Filter(playerList, func(item string, index int) bool {
		return len(item) >= len(targetName) && strings.EqualFold(targetName, item[:len(targetName)])
	})
	if len(playerList) != 1 {
		if len(playerList) == 0 {
			tp.Tellraw(player, []plugin.TellrawMessage{{Text: "找不到目标", Color: "red"}})
		} else {
			tp.Tellraw(player, []plugin.TellrawMessage{{Text: "非唯一目标", Color: "red"}})
		}
		return
	}
	go func() {
		tp.Tellraw(playerList[0], []plugin.TellrawMessage{{Text: "2秒后 ", Color: "green", Bold: true}, {Text: player, Color: "yellow"}, {Text: " TP至你", Color: "green", Bold: true}})
		tp.Tellraw(player, []plugin.TellrawMessage{{Text: "2秒后TP至 ", Color: "green", Bold: true}, {Text: playerList[0], Color: "yellow", Bold: true}})
	}()
	err := tp.teleportCore.Teleport(player, playerList[0])
	if err != nil {
		tp.Tellraw(player, []plugin.TellrawMessage{{Text: err.Error(), Color: "red"}})
	}
}

func (tp *TeleportPlugin) Start() {
}
