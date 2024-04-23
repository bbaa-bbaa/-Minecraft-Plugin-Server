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
	"context"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/plugin/pluginabi"
	"github.com/fatih/color"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GameManagerMessageBus struct {
	client   manager.Manager_MessageClient
	channels []chan *pluginabi.GameManagerMessage
	lock     sync.RWMutex
}

type MinecraftPluginManager struct {
	Address          string
	StartScript      string
	ClientInfo       *manager.Client
	client           manager.ManagerClient
	context          context.Context
	messageBus       GameManagerMessageBus
	commandProcessor *MinecraftCommandProcessor
	plugins          map[string]pluginabi.Plugin
	pluginLock       sync.RWMutex
	minecraftState   manager.MinecraftState
}

func (mpm *MinecraftPluginManager) RunCommand(cmd string) string {
	return mpm.commandProcessor.RunCommand(cmd)
}
func (mpm *MinecraftPluginManager) Lock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mpm.client.Lock(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Unlock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mpm.client.Unlock(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Write(wr *manager.WriteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	wr.Client = mpm.ClientInfo
	return mpm.client.Write(mpm.context, wr, opts...)
}
func (mpm *MinecraftPluginManager) Start(st *manager.StartRequest, opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	st.Client = mpm.ClientInfo
	return mpm.client.Start(mpm.context, st, opts...)
}
func (mpm *MinecraftPluginManager) Stop(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mpm.client.Stop(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Status(opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	return mpm.client.Status(mpm.context, mpm.ClientInfo, opts...)
}

func (mpm *MinecraftPluginManager) Printf(scope string, format string, a ...any) (n int, err error) {

	return fmt.Printf(color.YellowString("[")+"%s"+color.YellowString("] ")+strings.TrimRight(format, "\r\n")+"\r\n", append([]any{scope}, a...)...)
}

func (mpm *MinecraftPluginManager) Println(scope string, a ...any) (n int, err error) {
	return mpm.Printf(scope, "%s", strings.TrimRight(fmt.Sprint(a...), "\r\n"))
}

func (mpm *MinecraftPluginManager) kPrintln(a ...any) (n int, err error) {
	return mpm.Println(color.RedString("MinecraftManager"), a...)
}

func (mpm *MinecraftPluginManager) login() (err error) {
	mpm.ClientInfo, err = mpm.client.Login(mpm.context, nil)
	if err != nil {
		mpm.kPrintln(color.RedString("获取 Client ID 失败: " + err.Error()))
		return err
	}
	mpm.kPrintln(color.YellowString("从 GameManager 获取 ClientId:%s ", color.GreenString("%d", mpm.ClientInfo.Id)))
	return nil
}

func (mpm *MinecraftPluginManager) messageForwardWorker() {
	mpm.kPrintln(color.YellowString("消息转发 Worker 启动"))
	for {
		message, err := mpm.messageBus.client.Recv()
		if err != nil {
			mpm.kPrintln(color.RedString("MessageBus 关闭"))
			break
		}
		coreMessage := &pluginabi.GameManagerMessage{Message: message, CommandRespone: mpm.commandProcessor.responeReceivers != nil}
		mpm.messageBus.lock.RLock()
		for _, channel := range mpm.messageBus.channels {
			select {
			case channel <- coreMessage:
			default:
			}
		}
		mpm.messageBus.lock.RUnlock()
	}
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (mpm *MinecraftPluginManager) RegisterLogProcesser(context pluginabi.PluginName, process func(string, bool)) (channel chan *pluginabi.GameManagerMessage) {
	return mpm.registerLogProcesser(context, process, false)
}

func (mpm *MinecraftPluginManager) registerLogProcesser(context pluginabi.PluginName, process func(string, bool), skipRegister bool) (channel chan *pluginabi.GameManagerMessage) {
	var pluginName string
	if context == nil {
		pluginName = "anonymous"
	} else {
		pluginName = context.DisplayName()
	}
	mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginName), color.YellowString(" 注册了一个日志处理器: "), color.GreenString(GetFunctionName(process)))
	channel = mpm.RegisterServerMessageProcesser(skipRegister)
	go func() {
		for msg := range channel {
			switch msg.Message.Type {
			case "stdout":
				process(msg.Message.Content, msg.CommandRespone)
			}
		}
	}()
	return channel
}

func (mpm *MinecraftPluginManager) registerServerMessageListener() (err error) {
	mpm.messageBus.client, err = mpm.client.Message(mpm.context, mpm.ClientInfo)
	if err != nil {
		mpm.kPrintln(color.RedString("无法注册服务消息侦听器，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	go mpm.messageForwardWorker()
	return nil
}

func (mpm *MinecraftPluginManager) RegisterServerMessageProcesser(skipRegister bool) (channel chan *pluginabi.GameManagerMessage) {
	mpm.messageBus.lock.Lock()
	defer mpm.messageBus.lock.Unlock()
	channel = make(chan *pluginabi.GameManagerMessage, 16384)
	if !skipRegister {
		mpm.messageBus.channels = append(mpm.messageBus.channels, channel)
	}
	return channel
}

func (mpm *MinecraftPluginManager) UnregisterServerMessageProcesser(channel chan *pluginabi.GameManagerMessage) {
	mpm.messageBus.lock.Lock()
	defer mpm.messageBus.lock.Unlock()
	idx := slices.Index(mpm.messageBus.channels, channel)
	if idx >= 0 {
		mpm.messageBus.channels = slices.Delete(mpm.messageBus.channels, idx, idx+1)
	}
}

func (mpm *MinecraftPluginManager) getStatus() (status *manager.StatusResponse, err error) {
	status, err = mpm.Status()
	if err != nil {
		mpm.kPrintln(color.RedString("无法获取服务器状态: %s", err.Error()))
		return nil, err
	}
	return status, nil
}

func (mpm *MinecraftPluginManager) StartMinecraft() (err error) {
	_, err = mpm.Start(&manager.StartRequest{Client: mpm.ClientInfo, Path: mpm.StartScript})
	if err != nil {
		mpm.kPrintln(color.RedString("Minecraft 服务器启动失败: %s", err.Error()))
		return err
	}
	return nil
}

func (mpm *MinecraftPluginManager) RegisterPlugin(plugin pluginabi.Plugin) (err error) {
	pluginName := plugin.Name()
	pluginDisplayName := plugin.DisplayName()
	mpm.pluginLock.Lock()
	if _, ok := mpm.plugins[pluginName]; !ok {
		mpm.plugins[pluginName] = plugin
		mpm.pluginLock.Unlock()
		mpm.kPrintln(color.YellowString("注册并加载新插件 "), color.BlueString(pluginDisplayName))
		err := plugin.Init(mpm)
		if err != nil {
			mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginDisplayName), color.RedString(" 加载失败: "), color.MagentaString(err.Error()))
		}
		mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginDisplayName), color.GreenString(" 加载成功"))
		if mpm.minecraftState == manager.MinecraftState_running {
			plugin.Start()
		}
	} else {
		mpm.pluginLock.Unlock()
		mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginDisplayName), color.RedString(" 已经加载，"), color.YellowString("本次加载请求忽略"))
	}

	return nil
}

func (mpm *MinecraftPluginManager) GetPlugin(pluginName string) pluginabi.Plugin {
	mpm.pluginLock.RLock()
	defer mpm.pluginLock.RUnlock()
	if plugin, ok := mpm.plugins[pluginName]; ok {
		return plugin
	}
	return nil
}

func (mpm *MinecraftPluginManager) pluginStart() {
	mpm.pluginLock.RLock()
	for _, plugin := range mpm.plugins {
		plugin.Start()
	}
	mpm.pluginLock.RUnlock()
}

func (mpm *MinecraftPluginManager) pluginPause() {
	mpm.pluginLock.RLock()
	for _, plugin := range mpm.plugins {
		plugin.Pause()
	}
	mpm.pluginLock.RUnlock()
}

func (mpm *MinecraftPluginManager) loadBulitinPlugin() {
	mpm.RegisterPlugin(&plugin.ScoreboardCore{})
	mpm.RegisterPlugin(&plugin.PlayerInfo{})
	mpm.RegisterPlugin(&plugin.TeleportCore{})
	mpm.RegisterPlugin(&plugin.SimpleCommand{})
}

func (mpm *MinecraftPluginManager) initClient() (err error) {
	mpm.kPrintln(color.YellowString("正在登录 Manager Backend"))
	err = mpm.login()
	if err != nil {
		return err
	}
	mpm.kPrintln(color.YellowString("正在注册 MessageBus/Stdout 转发器"))
	err = mpm.registerServerMessageListener()
	if err != nil {
		return err
	}
	mpm.kPrintln(color.YellowString("正在注册命令处理器"))
	mpm.commandProcessor = &MinecraftCommandProcessor{}
	mpm.RegisterPlugin(mpm.commandProcessor)
	mpm.kPrintln(color.YellowString("正在加载内置插件"))
	mpm.loadBulitinPlugin()
	mpm.kPrintln(color.YellowString("正在获取服务器状态"))
	status, err := mpm.getStatus()
	if err != nil {
		return err
	}
	switch status.State {
	case manager.MinecraftState_running:
		mpm.kPrintln(color.GreenString("Minecraft 进程已经运行，检测并等待启动完成"))
		minecraftStartingLog := mpm.RegisterLogProcesser(&pluginabi.PluginNameWrapper{PluginName: "Minecraft启动日志"}, func(s string, _ bool) {
			mpm.kPrintln(color.YellowString("服务器日志: "), color.CyanString(s))
		})
		mpm.RunCommand("testServerReady")
		mpm.kPrintln(color.GreenString("Minecraft 启动成功"))
		mpm.UnregisterServerMessageProcesser(minecraftStartingLog)
		close(minecraftStartingLog)
	case manager.MinecraftState_stopped:
		mpm.kPrintln(color.YellowString("正在启动 Minecraft 服务器"))
		err = mpm.StartMinecraft()
		if err != nil {
			return err
		}
		mpm.kPrintln(color.YellowString("Minecraft 启动请求发送"))
		minecraftStartingLog := mpm.RegisterLogProcesser(&pluginabi.PluginNameWrapper{PluginName: "Minecraft启动日志"}, func(s string, _ bool) {
			mpm.kPrintln(color.YellowString("服务器日志: "), color.CyanString(s))
		})
		mpm.RunCommand("testServerReady")
		mpm.kPrintln(color.GreenString("Minecraft 启动成功"))
		mpm.UnregisterServerMessageProcesser(minecraftStartingLog)
		close(minecraftStartingLog)
	}
	mpm.minecraftState = manager.MinecraftState_running
	mpm.kPrintln(color.YellowString("通知插件 Minecraft 启动完成"))
	mpm.pluginStart()
	return nil
}

func NewPluginManager() (pm *MinecraftPluginManager) {
	pm = &MinecraftPluginManager{plugins: make(map[string]pluginabi.Plugin)}
	return pm
}

func (mpm *MinecraftPluginManager) init() (err error) {
	if mpm.plugins == nil {
		mpm.plugins = make(map[string]pluginabi.Plugin)
	}
	return mpm.initClient()
}

func (mpm *MinecraftPluginManager) Dial(server string) (err error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	mpm.Address = server
	conn, err := grpc.Dial(mpm.Address, opts...)
	if err != nil {
		mpm.kPrintln(color.RedString("无法连接上 Manager Backend，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	mpm.client = manager.NewManagerClient(conn)
	mpm.context = context.Background()

	return mpm.init()
}
