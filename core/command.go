package core

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"github.com/fatih/color"
)

type MinecraftCommandRequest struct {
	command  string
	response chan string
}

type MinecraftCommandProcessor struct {
	managerClient    *MinecraftManagerClient
	queue            chan *MinecraftCommandRequest
	responeReceivers chan string
	index            uint64
}

var SkipWaitCommand []string = []string{"tellraw"}
var WaitForRegexCommand map[string]*regexp.Regexp = map[string]*regexp.Regexp{"save-all": regexp.MustCompile("Saved")}

func (mc *MinecraftCommandProcessor) kPrintln(a ...any) (int, error) {
	return mc.managerClient.Println(color.MagentaString("CommandProcessor"), a...)
}

func NewCommandProcessor(client *MinecraftManagerClient) *MinecraftCommandProcessor {
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
		mc.responeReceivers = make(chan string, 32)
		command := strings.Split(cmd.command, " ")[0]
		mc.kPrintln(color.YellowString("正在执行命令["), color.CyanString("%d", mc.index), color.YellowString("]: "), color.BlueString(cmd.command))
		mc.managerClient.Write(&manager.WriteRequest{Id: mc.index, Content: cmd.command})
		if slices.Index(SkipWaitCommand, command) >= 0 {
			cmd.response <- ""
			mc.responeReceivers = nil
			mc.managerClient.Unlock()
			mc.index++
			continue
		}
		endCommandTimer := time.NewTimer(1 * time.Second)
		if waitRegex, ok = WaitForRegexCommand[command]; ok {
			endCommandTimer.C = nil
		}
	cmdReceiver:
		for {
			select {
			case line := <-mc.responeReceivers:
				endCommandTimer.Reset(15 * time.Millisecond)
				match := DedicatedServerMessage.FindStringSubmatch(line)
				if len(match) == 2 {
					commandBuffer = append(commandBuffer, match[1])
					mc.kPrintln(color.GreenString("将命令["), color.CyanString("%d", mc.index), color.YellowString("]: "), color.BlueString(cmd.command), color.GreenString(" 的输出储存为:"), color.YellowString(match[1]))
					if waitRegex != nil && waitRegex.MatchString(match[1]) {
						break cmdReceiver
					}
				}
			case <-endCommandTimer.C:
				mc.responeReceivers = nil
				break cmdReceiver
			}
		}
		cmd.response <- strings.Join(commandBuffer, "\n")
		mc.managerClient.Unlock()
		mc.index++
	}
}

func (mc *MinecraftCommandProcessor) Init() {
	mc.managerClient.registerLogProcesser(mc.commandResponeProcessor)
	go mc.Worker()
}
