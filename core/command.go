package core

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"github.com/fatih/color"
)

type MinecraftCommandRequest struct {
	command  string
	response chan string
}

type MinecraftCommandProcessor struct {
	managerClient    *MinecraftPluginManager
	queue            chan *MinecraftCommandRequest
	responeReceivers chan string
	receiverLock     sync.RWMutex
	index            uint64
}

var SkipWaitCommand []string = []string{"tellraw"}
var WaitForRegexCommand map[string]*regexp.Regexp = map[string]*regexp.Regexp{"save-all": regexp.MustCompile("Saved"), "testServerReady": regexp.MustCompile("Unknown or incomplete command")}

func (mc *MinecraftCommandProcessor) Println(a ...any) (int, error) {
	return mc.managerClient.Println(color.MagentaString("CommandProcessor"), a...)
}

func NewCommandProcessor(client *MinecraftPluginManager) *MinecraftCommandProcessor {
	cmd := &MinecraftCommandProcessor{
		managerClient: client,
		queue:         make(chan *MinecraftCommandRequest, 16384),
	}
	cmd.Init()
	return cmd
}

func (mc *MinecraftCommandProcessor) RunCommand(command string) (response string) {
	resp := make(chan string, 1)
	mc.queue <- &MinecraftCommandRequest{
		command:  command,
		response: resp,
	}
	return <-resp
}

func (mc *MinecraftCommandProcessor) commandResponeProcessor(logText string) {
	mc.receiverLock.RLock()
	defer mc.receiverLock.RUnlock()
	if mc.responeReceivers != nil {
		if DedicatedServerMessage.MatchString(logText) && !PlayerMessage.MatchString(logText) &&
			!GameLeftMessage.MatchString(logText) && !LoginMessage.MatchString(logText) {
			mc.responeReceivers <- logText
		}
	}
}

func (mc *MinecraftCommandProcessor) Worker() {
	for cmd := range mc.queue {
		var waitRegex *regexp.Regexp
		var ok bool
		commandBuffer := make([]string, 0, 32)
		mc.managerClient.Lock()
		responseReceiver := make(chan string, 32)
		mc.receiverLock.Lock()
		mc.responeReceivers = responseReceiver
		mc.receiverLock.Unlock()
		cmd.command = strings.TrimLeft(cmd.command, "/")
		command := strings.Split(cmd.command, " ")[0]
		mc.Println(color.YellowString("正在执行命令["), color.CyanString("%d", mc.index), color.YellowString("]: "), color.BlueString(cmd.command), color.YellowString(" 队列中剩余: "), color.RedString("%d", len(mc.queue)))
		mc.managerClient.Write(&manager.WriteRequest{Id: mc.index, Content: cmd.command})
		if slices.Index(SkipWaitCommand, command) >= 0 {
			cmd.response <- ""
			mc.responeReceivers = nil
			mc.managerClient.Unlock()
			mc.index++
			continue
		}
		renewLockTicker := time.NewTicker(5 * time.Second)
		endCommandTimer := time.NewTimer(100 * time.Millisecond)
		if waitRegex, ok = WaitForRegexCommand[command]; ok {
			endCommandTimer.C = nil
		}
	cmdReceiver:
		for {
			select {
			case line := <-responseReceiver:
				endCommandTimer.Reset(10 * time.Millisecond)
				match := DedicatedServerMessage.FindStringSubmatch(line)
				if len(match) == 2 {
					commandBuffer = append(commandBuffer, match[1])
					if waitRegex == nil {
						mc.Println(color.YellowString("将命令["), color.CyanString("%d", mc.index), color.YellowString("]: "), color.BlueString(cmd.command), color.YellowString(" 的输出储存为:"), color.YellowString(match[1]))
					}
					if waitRegex != nil && waitRegex.MatchString(match[1]) {
						mc.Println(color.YellowString("将命令["), color.CyanString("%d", mc.index), color.YellowString("]: "), color.BlueString(cmd.command), color.YellowString(" 的输出储存为:"), color.YellowString(match[1]))
						break cmdReceiver
					}
				}
			case <-endCommandTimer.C:
				mc.receiverLock.Lock()
				mc.responeReceivers = nil
				mc.receiverLock.Unlock()
				break cmdReceiver
			case <-renewLockTicker.C:
				mc.managerClient.Lock()
			}
		}
		renewLockTicker.Stop()
		cmd.response <- strings.Join(commandBuffer, "\n")
		mc.managerClient.Unlock()
		mc.index++
	}
}

func (mc *MinecraftCommandProcessor) Init() {
	mc.managerClient.RegisterLogProcesser(mc, mc.commandResponeProcessor)
	go mc.Worker()
}

func (mc *MinecraftCommandProcessor) Name() string {
	return "CommandProcessor"
}

func (mc *MinecraftCommandProcessor) DisplayName() string {
	return "命令处理器"
}
