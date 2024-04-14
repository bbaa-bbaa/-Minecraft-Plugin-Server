package plugin

import (
	"regexp"
	"strings"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
)

type SimpleCommand struct {
	BasePlugin
	playerCommand    *regexp.Regexp
	registerCommands map[string]func(string, ...string)
}

func (sp *SimpleCommand) RegisterCommand(command string, commandFunc func(string, ...string)) {
	if commandFunc, ok := sp.registerCommands[command]; !ok {
		sp.registerCommands[command] = commandFunc
	}
}

func (sp *SimpleCommand) Init(mpm pluginabi.PluginManager) {
	sp.BasePlugin.Init(mpm)
	mpm.RegisterLogProcesser(sp, sp.processCommand)
	sp.playerCommand = regexp.MustCompile(`.*?\]:.*?<(.*?)>.*?!!(.*)`)
}

func (sp *SimpleCommand) processCommand(logText string) {
	cmdInfo := sp.playerCommand.FindStringSubmatch(logText)
	if len(cmdInfo) < 3 {
		return
	}
	player := strings.TrimSpace(cmdInfo[1])
	rawCommand := strings.TrimSpace(cmdInfo[2])
	commandPart := strings.Split(rawCommand, " ")
	command := commandPart[0]
	if commandFunc, ok := sp.registerCommands[command]; ok {
		commandFunc(player, commandPart[1:]...)
	}
}

func (sp *SimpleCommand) Name() string {
	return "简单命令"
}
