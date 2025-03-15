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

package core

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/manager"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
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
	cleanSignal      chan struct{}
}

func (mc *MinecraftCommandProcessor) Println(a ...any) (int, error) {
	return mc.managerClient.Println(color.MagentaString(mc.DisplayName()), a...)
}

var UnknownCommand = regexp.MustCompile("Unknown or incomplete command")

var SkipWaitCommand []string = []string{"tellraw"}
var WaitForRegexCommand map[string]*regexp.Regexp = map[string]*regexp.Regexp{"save-all": regexp.MustCompile("Saved"), "testServerReady": UnknownCommand, "list": regexp.MustCompile("players online")}

func (mc *MinecraftCommandProcessor) RunCommand(command string) (response string) {
	resp := make(chan string, 1)
	mc.queue <- &MinecraftCommandRequest{
		command:  command,
		response: resp,
	}
	return <-resp
}

func (mc *MinecraftCommandProcessor) commandResponeProcessor(logText string, _ bool) {
	mc.receiverLock.RLock()
	receiver := mc.responeReceivers
	mc.receiverLock.RUnlock()
	if receiver != nil {
		if DedicatedServerMessage.MatchString(logText) && !PlayerMessage.MatchString(logText) &&
			!PlayerJoinLeaveMessage.MatchString(logText) && !LoginMessage.MatchString(logText) && !PlayerCommandMessage.MatchString(logText) {
			receiver <- logText
		}
	}
}

func (mc *MinecraftCommandProcessor) Worker() {
	for cmd := range mc.queue {
		var waitRegex *regexp.Regexp
		var isWaitRegex bool
		var responseReceiver chan string
		var cleanSignal chan struct{}
		commandBuffer := make([]string, 0, 32)
		_, err := mc.managerClient.Lock()
		if err != nil {
			cmd.response <- ""
			mc.managerClient.Unlock()
			mc.index++
			continue
		}
		cmd.command = strings.TrimLeft(cmd.command, "/")
		command := strings.Split(cmd.command, " ")[0]
		mc.Println(color.YellowString("正在执行命令["), color.GreenString("%d", mc.index), color.YellowString("]: "), color.RedString(cmd.command), color.YellowString(" 队列中剩余: "), color.RedString("%d", len(mc.queue)))
		if slices.Index(SkipWaitCommand, command) < 0 {
			responseReceiver = make(chan string, 32)
			mc.receiverLock.Lock()
			mc.responeReceivers = responseReceiver
			cleanSignal = make(chan struct{})
			mc.cleanSignal = cleanSignal
			mc.receiverLock.Unlock()
			mc.managerClient.Write(&manager.WriteRequest{Id: mc.index, Content: cmd.command})
		} else {
			mc.managerClient.Write(&manager.WriteRequest{Id: mc.index, Content: cmd.command})
			cmd.response <- ""
			mc.managerClient.Unlock()
			mc.index++
			continue
		}
		renewLockTicker := time.NewTicker(5 * time.Second)
		var endCommandTimer *time.Timer
		var endCommandChannel <-chan time.Time = nil
		if waitRegex, isWaitRegex = WaitForRegexCommand[command]; !isWaitRegex {
			endCommandTimer = time.NewTimer(100 * time.Millisecond)
			endCommandChannel = endCommandTimer.C
		}

	cmdReceiver:
		for {
			select {
			case <-renewLockTicker.C:
				mc.managerClient.Lock()
			case line, ok := <-responseReceiver:
				if !ok {
					continue
				}
				queue := len(responseReceiver)
				if !isWaitRegex {
					if !endCommandTimer.Stop() {
						<-endCommandTimer.C
					}
					endCommandTimer.Reset(10*time.Millisecond + time.Duration(10*queue)*time.Millisecond)
				}
				match := DedicatedServerMessage.FindStringSubmatch(line)
				if len(match) == 2 {
					commandBuffer = append(commandBuffer, match[1])
					if !isWaitRegex {
						mc.Println(color.YellowString("将命令["), color.GreenString("%d", mc.index), color.YellowString("]: "), color.RedString(cmd.command), color.YellowString(" 的输出储存为: "), color.CyanString(match[1]))
					} else if waitRegex.MatchString(match[1]) {
						mc.Println(color.YellowString("将命令["), color.GreenString("%d", mc.index), color.YellowString("]: "), color.RedString(cmd.command), color.YellowString(" 的输出储存为: "), color.CyanString(match[1]))
						endCommandTimer = time.NewTimer(10*time.Millisecond + time.Duration(10*queue)*time.Millisecond)
						endCommandChannel = endCommandTimer.C
						isWaitRegex = false
					}
				}
			case <-endCommandChannel:
				mc.Println(color.BlueString("命令执行结束"), color.YellowString("["), color.GreenString("%d", mc.index), color.YellowString("]: "), color.RedString(cmd.command))
				break cmdReceiver
			case <-cleanSignal:
				mc.Println(color.RedString("清理未完成的命令: "), color.YellowString(command))
				break cmdReceiver
			}
		}
		mc.receiverLock.Lock()
		mc.responeReceivers = nil
		mc.cleanSignal = nil
		mc.receiverLock.Unlock()
		renewLockTicker.Stop()
		if endCommandTimer != nil {
			endCommandTimer.Stop()
		}
		cmd.response <- strings.Join(commandBuffer, "\n")
		mc.managerClient.Unlock()
		mc.index++
	}
}

func (mc *MinecraftCommandProcessor) Init(mpm pluginabi.PluginManager) error {
	mc.managerClient = mpm.(*MinecraftPluginManager)
	mpm.RegisterLogProcesser(mc, mc.commandResponeProcessor)
	mc.queue = make(chan *MinecraftCommandRequest, 16384)
	go mc.Worker()
	return nil
}

func (mc *MinecraftCommandProcessor) Name() string {
	return "CommandProcessor"
}

func (mc *MinecraftCommandProcessor) DisplayName() string {
	return "命令处理器"
}

func (mc *MinecraftCommandProcessor) Start() {
}

func (mc *MinecraftCommandProcessor) Pause() {
	mc.receiverLock.RLock()
	defer mc.receiverLock.RUnlock()
	if mc.cleanSignal != nil {
		mc.cleanSignal <- struct{}{}
	}
}
