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
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/manager"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"github.com/fatih/color"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GameManagerMessageBus struct {
	client   manager.Manager_MessageClient
	channels []chan *manager.MessageResponse
	lock     sync.RWMutex
}

type PluginManager struct {
	started bool
	plugin  pluginabi.Plugin
}

func (pm *PluginManager) Init(mpm *MinecraftPluginManager) error {
	mpm.kPrintln(color.YellowString("加载插件 "), color.BlueString(pm.plugin.DisplayName()))
	err := pm.plugin.Init(mpm)
	if err != nil {
		mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pm.plugin.DisplayName()), color.RedString(" 加载失败: "), color.MagentaString(err.Error()))
		return err
	}
	mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pm.plugin.DisplayName()), color.GreenString(" 加载成功"))
	if mpm.minecraftState == manager.MinecraftState_running {
		pm.Start()
	}
	return nil
}

func (pm *PluginManager) Start() {
	if pm.plugin != nil && !pm.started {
		pm.started = true
		pm.plugin.Start()
	}
}

func (pm *PluginManager) Pause() {
	if pm.plugin != nil && pm.started {
		pm.started = false
		pm.plugin.Pause()
	}
}

var (
	errGameServerStopped     = fmt.Errorf("minecraft game stop")
	errGrpcChannelDisconnect = fmt.Errorf("grpc disconnected")
)

type MinecraftPluginManager struct {
	Repl             *REPLPlugin
	Address          string
	StartScript      string
	ClientInfo       *manager.Client
	client           manager.ManagerClient
	context          context.Context
	messageBus       GameManagerMessageBus
	errBus           chan error
	commandProcessor *MinecraftCommandProcessor
	plugins          map[string]*PluginManager
	delayinitPlugins []*PluginManager
	pluginLock       sync.RWMutex
	minecraftState   manager.MinecraftState
}

func (mpm *MinecraftPluginManager) RunCommand(cmd string) string {
	return mpm.commandProcessor.RunCommand(cmd)
}
func (mpm *MinecraftPluginManager) Lock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	return mpm.client.Lock(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Unlock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	return mpm.client.Unlock(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Write(wr *manager.WriteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	wr.Client = mpm.ClientInfo
	return mpm.client.Write(mpm.context, wr, opts...)
}
func (mpm *MinecraftPluginManager) Start(st *manager.StartRequest, opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	st.Client = mpm.ClientInfo
	return mpm.client.Start(mpm.context, st, opts...)
}
func (mpm *MinecraftPluginManager) Stop(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	return mpm.client.Stop(mpm.context, mpm.ClientInfo, opts...)
}
func (mpm *MinecraftPluginManager) Status(opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	if mpm.ClientInfo == nil {
		return nil, errGrpcChannelDisconnect
	}
	return mpm.client.Status(mpm.context, mpm.ClientInfo, opts...)
}

func (mpm *MinecraftPluginManager) Printf(scope string, format string, a ...any) (n int, err error) {
	if mpm.Repl != nil && mpm.Repl.terminal != nil {
		s := fmt.Sprintf(color.YellowString("[")+"%s"+color.YellowString("] ")+strings.TrimRight(format, "\r\n")+"\r\n", append([]any{scope}, a...)...)
		mpm.Repl.terminal.Write([]byte(s))
	} else {
		n, err = fmt.Printf(color.YellowString("[")+"%s"+color.YellowString("] ")+strings.TrimRight(format, "\r\n")+"\r\n", append([]any{scope}, a...)...)
	}
	return
}

func (mpm *MinecraftPluginManager) Println(scope string, a ...any) (n int, err error) {
	return mpm.Printf(scope, "%s", strings.TrimRight(fmt.Sprint(a...), "\r\n"))
}

func (mpm *MinecraftPluginManager) kPrintln(a ...any) (n int, err error) {
	return mpm.Println(color.RedString("MinecraftManager"), a...)
}

func (mpm *MinecraftPluginManager) login(waitForReady bool) (err error) {
	mpm.ClientInfo, err = mpm.client.Login(mpm.context, nil, grpc.WaitForReady(waitForReady))
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
			if mpm.errBus != nil {
				mpm.errBus <- errGrpcChannelDisconnect
			}
			break
		}
		mpm.messageBus.lock.RLock()
		for _, channel := range mpm.messageBus.channels {
			select {
			case channel <- message:
			default:
			}
		}
		mpm.messageBus.lock.RUnlock()
	}
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (mpm *MinecraftPluginManager) RegisterLogProcesser(context pluginabi.PluginName, process func(string, bool)) (channel chan *manager.MessageResponse) {
	return mpm.registerLogProcesser(context, process, false)
}

func (mpm *MinecraftPluginManager) registerLogProcesser(context pluginabi.PluginName, process func(string, bool), skipRegister bool) (channel chan *manager.MessageResponse) {
	var pluginName string
	if context == nil {
		pluginName = "anonymous"
	} else {
		pluginName = context.DisplayName()
	}
	mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginName), color.YellowString(" 注册了一个日志处理器: "), color.GreenString(GetFunctionName(process)))
	channel = mpm.RegisterManagerMessageChannel(skipRegister)
	go func() {
		for msg := range channel {
			switch msg.Type {
			case "stdout":
				process(msg.Content, msg.Locked)
			}
		}
	}()
	return channel
}

func (mpm *MinecraftPluginManager) registerServerMessageListener(waitForReady bool) (err error) {
	mpm.messageBus.client, err = mpm.client.Message(mpm.context, mpm.ClientInfo, grpc.WaitForReady(waitForReady))
	if err != nil {
		mpm.kPrintln(color.RedString("无法注册服务消息侦听器，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	go mpm.messageForwardWorker()
	return nil
}

func (mpm *MinecraftPluginManager) RegisterManagerMessageChannel(skipRegister bool) (channel chan *manager.MessageResponse) {
	mpm.messageBus.lock.Lock()
	defer mpm.messageBus.lock.Unlock()
	channel = make(chan *manager.MessageResponse, 16384)
	if !skipRegister {
		mpm.messageBus.channels = append(mpm.messageBus.channels, channel)
	}
	return channel
}

func (mpm *MinecraftPluginManager) UnRegisterManagerMessageChannel(channel chan *manager.MessageResponse) {
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

func (mpm *MinecraftPluginManager) startMinecraft() (err error) {
	_, err = mpm.Start(&manager.StartRequest{Client: mpm.ClientInfo, Path: mpm.StartScript})
	if err != nil {
		mpm.kPrintln(color.RedString("Minecraft 服务器启动失败: %s", err.Error()))
		return err
	}
	return nil
}

func (mpm *MinecraftPluginManager) loadPlugin(plugin pluginabi.Plugin, init bool) (p pluginabi.Plugin, err error) {
	pluginName := plugin.Name()
	pluginDisplayName := plugin.DisplayName()
	mpm.pluginLock.Lock()
	if _, ok := mpm.plugins[pluginName]; !ok {
		pm := &PluginManager{plugin: plugin}
		mpm.plugins[pluginName] = pm
		mpm.pluginLock.Unlock()
		mpm.kPrintln(color.YellowString("注册新插件 "), color.BlueString(pluginDisplayName))
		if init {
			err = pm.Init(mpm)
			if err != nil {
				return plugin, err
			}
		} else {
			mpm.delayinitPlugins = append(mpm.delayinitPlugins, pm)
		}
	} else {
		mpm.pluginLock.Unlock()
		mpm.kPrintln(color.YellowString("插件 "), color.BlueString(pluginDisplayName), color.RedString(" 已经注册"))
	}

	return plugin, nil
}

func (mpm *MinecraftPluginManager) registerPlugin(plugin pluginabi.Plugin) (p pluginabi.Plugin, err error) {
	return mpm.loadPlugin(plugin, false)
}

func (mpm *MinecraftPluginManager) RegisterPlugin(plugin pluginabi.Plugin) (p pluginabi.Plugin, err error) {
	return mpm.loadPlugin(plugin, true)
}

func (mpm *MinecraftPluginManager) GetPlugin(pluginName string) pluginabi.Plugin {
	mpm.pluginLock.RLock()
	defer mpm.pluginLock.RUnlock()
	if plugin, ok := mpm.plugins[pluginName]; ok {
		return plugin.plugin
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

func (mpm *MinecraftPluginManager) StartMinecraft() (err error) {
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
		mpm.UnRegisterManagerMessageChannel(minecraftStartingLog)
		close(minecraftStartingLog)
	case manager.MinecraftState_stopped:
		mpm.kPrintln(color.YellowString("正在启动 Minecraft 服务器"))
		err = mpm.startMinecraft()
		if err != nil {
			return err
		}
		mpm.kPrintln(color.YellowString("Minecraft 启动请求发送"))
		minecraftStartingLog := mpm.RegisterLogProcesser(&pluginabi.PluginNameWrapper{PluginName: "Minecraft启动日志"}, func(s string, _ bool) {
			mpm.kPrintln(color.YellowString("服务器日志: "), color.CyanString(s))
		})
		mpm.RunCommand("testServerReady")
		mpm.kPrintln(color.GreenString("Minecraft 启动成功"))
		mpm.UnRegisterManagerMessageChannel(minecraftStartingLog)
		close(minecraftStartingLog)
	}
	mpm.minecraftState = manager.MinecraftState_running
	mpm.kPrintln(color.YellowString("通知插件 Minecraft 启动完成"))
	mpm.pluginStart()
	return nil
}

func (mpm *MinecraftPluginManager) initDelayedPlugin() {
	mpm.pluginLock.Lock()
	plugins := slices.Clone(mpm.delayinitPlugins)
	mpm.delayinitPlugins = nil
	mpm.pluginLock.Unlock()
	for _, pm := range plugins {
		pm.Init(mpm)
	}

	mpm.pluginLock.Lock()
	mpm.delayinitPlugins = nil
	mpm.pluginLock.Unlock()
}

func (mpm *MinecraftPluginManager) initPlugin() (err error) {
	mpm.kPrintln(color.YellowString("正在注册命令处理器"))
	mpm.commandProcessor = &MinecraftCommandProcessor{}
	mpm.RegisterPlugin(mpm.commandProcessor)
	mpm.kPrintln(color.YellowString("正在加载内置插件"))
	// repl
	mpm.Repl = &REPLPlugin{}
	mpm.RegisterPlugin(mpm.Repl)

	mpm.registerPlugin(&plugin.ScoreboardCore{})
	mpm.registerPlugin(&plugin.TellrawManager{})
	mpm.registerPlugin(&plugin.PlayerInfo{})
	mpm.registerPlugin(&plugin.TeleportCore{})
	mpm.registerPlugin(&plugin.SimpleCommand{})
	mpm.initDelayedPlugin()
	return
}

func (mpm *MinecraftPluginManager) initClient(waitForReady bool) (err error) {
	mpm.kPrintln(color.YellowString("正在登录 Manager Backend"))
	err = mpm.login(waitForReady)
	if err != nil {
		return err
	}
	mpm.kPrintln(color.YellowString("正在注册 MessageBus/Stdout 转发器"))
	err = mpm.registerServerMessageListener(waitForReady)
	if err != nil {
		return err
	}
	return nil
}
func (mpm *MinecraftPluginManager) monitorGameStopWorker(message chan *manager.MessageResponse) {
	for msg := range message {
		switch msg.Type {
		case "StateChange":
			switch msg.Content {
			case "GameServerStop":
				mpm.errBus <- errGameServerStopped
			}
		}
	}
}

func (mpm *MinecraftPluginManager) errorHandler() {
	for err := range mpm.errBus {
		switch err {
		case errGameServerStopped:
			mpm.kPrintln(color.RedString("服务器关闭，请求停止插件"))
			mpm.pluginPause()
		case errGrpcChannelDisconnect:
			mpm.ClientInfo = nil
			mpm.pluginPause()
			go func() {
				for {
					err := mpm.initClient(true)
					if err != nil {
						time.Sleep(5 * time.Second)
						continue
					}
					err = mpm.StartMinecraft()
					if err != nil {
						time.Sleep(5 * time.Second)
						continue
					}
					break
				}
			}()
		}
	}
}
func (mpm *MinecraftPluginManager) initErrorHandler() {
	if mpm.errBus != nil {
		return
	}
	mpm.errBus = make(chan error, 1)
	go mpm.errorHandler()
	go mpm.monitorGameStopWorker(mpm.RegisterManagerMessageChannel(false))
}

func (mpm *MinecraftPluginManager) initManager() (err error) {
	err = mpm.initClient(false)
	if err != nil {
		return err
	}
	mpm.initErrorHandler()
	err = mpm.initPlugin()
	if err != nil {
		return err
	}
	return mpm.StartMinecraft()
}

func NewPluginManager() (pm *MinecraftPluginManager) {
	pm = &MinecraftPluginManager{plugins: make(map[string]*PluginManager)}
	return pm
}

func (mpm *MinecraftPluginManager) init() (err error) {
	if mpm.plugins == nil {
		mpm.plugins = make(map[string]*PluginManager)
	}
	mpm.context = context.Background()
	return
}

func (mpm *MinecraftPluginManager) Dial(server string) (err error) {
	mpm.init()
	mpm.Address = server
	conn, err := grpc.NewClient(mpm.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		mpm.kPrintln(color.RedString("无法连接上 Manager Backend，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	mpm.client = manager.NewManagerClient(conn)

	return mpm.initManager()
}
