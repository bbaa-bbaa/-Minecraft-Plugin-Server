package plugin

import (
	"regexp"
	"strings"
	"sync"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"github.com/fatih/color"
)

type SimpleCommand struct {
	BasePlugin
	playerCommand    *regexp.Regexp
	registerCommands map[string]func(string, ...string)
	lock             sync.RWMutex
}

func (sp *SimpleCommand) RegisterCommand(context pluginabi.PluginName, command string, commandFunc func(string, ...string)) {
	sp.lock.Lock()
	defer sp.lock.Unlock()
	sp.Println(color.YellowString("插件 "), color.BlueString(context.DisplayName()), color.YellowString(" 注册了一条新命令: "), color.GreenString(command))
	if _, ok := sp.registerCommands[command]; !ok {
		sp.registerCommands[command] = commandFunc
	}
}

func (sp *SimpleCommand) Init(pm pluginabi.PluginManager) error {
	sp.BasePlugin.Init(pm, sp)
	pm.RegisterLogProcesser(sp, sp.processCommand)
	sp.playerCommand = regexp.MustCompile(`.*?\]:(?: \[[^\]]+\])? <(.*?)>.*?!!(.*)`)
	sp.registerCommands = make(map[string]func(string, ...string))
	return nil
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
	sp.lock.RLock()
	if commandFunc, ok := sp.registerCommands[command]; ok {
		sp.lock.RUnlock()
		go commandFunc(player, commandPart[1:]...)
	} else {
		sp.lock.RUnlock()
	}
}

func (sp *SimpleCommand) Name() string {
	return "SimpleCommand"
}

func (sp *SimpleCommand) DisplayName() string {
	return "简单命令"
}
