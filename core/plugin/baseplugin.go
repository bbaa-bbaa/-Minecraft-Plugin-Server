package plugin

import (
	"encoding/json"
	"fmt"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core"
	"github.com/fatih/color"
)

type BasePlugin struct {
	mpm *core.MinecraftPluginManager
}

type TellrawMessage struct {
	Text  string `json:"text"`
	Color string `json:"color"`
	Bold  bool   `json:"bold"`
}

func (bp *BasePlugin) Println(a ...any) (int, error) {
	return bp.mpm.Println(color.BlueString(bp.Name()), a...)
}

func (bp *BasePlugin) Init(mpm *core.MinecraftPluginManager) {
	bp.mpm = mpm
}

func (bp *BasePlugin) RunCommand(command string) string {
	return bp.mpm.RunCommand(command)
}

func (bp *BasePlugin) Tellraw(Target string, msg []TellrawMessage) string {
	jsonMsg, _ := json.Marshal(msg)
	return bp.mpm.RunCommand(fmt.Sprintf("/tellraw %s %s", Target, jsonMsg))
}

func (bp *BasePlugin) Name() string {
	return "基础插件"
}
