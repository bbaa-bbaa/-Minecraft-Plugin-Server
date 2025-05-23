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

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/manager"
	"github.com/fatih/color"
	"github.com/shirou/gopsutil/v3/process"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/emptypb"
)

func Printf(format string, a ...any) (n int, err error) {

	return fmt.Printf(color.YellowString("[")+color.RedString("GameManager")+color.YellowString("] ")+strings.TrimRight(format, "\r\n")+"\r\n", a...)
}

func Println(a ...any) (n int, err error) {

	return Printf("%s", strings.TrimRight(fmt.Sprint(a...), "\n"))
}

type MinecraftVistor struct {
	process *exec.Cmd
	pty     io.ReadWriteCloser
	state   manager.MinecraftState
}

type WriteLock struct {
	mutex        sync.Mutex
	lockedClient *manager.Client
	time         *time.Timer
	clientLock   sync.RWMutex
}

const lockMaxTime = 10 * time.Second

func (wl *WriteLock) Unlock(client *manager.Client) {
	wl.clientLock.Lock()
	defer wl.clientLock.Unlock()
	if wl.lockedClient != nil && wl.lockedClient.Id == client.Id {
		if wl.time != nil {
			wl.time.Stop()
			wl.time = nil
		}
		wl.lockedClient = nil
		wl.mutex.Unlock()
	}
}

func (wl *WriteLock) Lock(client *manager.Client, internal bool) {
	wl.clientLock.RLock()
	if wl.lockedClient != nil && client.Id == wl.lockedClient.Id {
		wl.time.Reset(lockMaxTime)
		if !internal {
			Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]续期写入锁"))
		}
		wl.clientLock.RUnlock()
		return
	} else {
		wl.clientLock.RUnlock()
	}
	wl.mutex.Lock()
	Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]获取写入锁"))
	wl.clientLock.Lock()
	wl.lockedClient = client
	wl.clientLock.Unlock()
	wl.time = time.AfterFunc(lockMaxTime, func() {
		Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]的写入锁因超时而被取消"))
		wl.Unlock(client)
	})
}

type ForwardChannelMessage struct {
	message string
	locked  bool
}

type ForwardChannel struct {
	channel chan *ForwardChannelMessage
	id      uint64
}
type ManagerServer struct {
	manager.UnimplementedManagerServer

	minecraftInstance  MinecraftVistor
	forwardWorker      bool
	forwardChannels    []*ForwardChannel
	forwardChannelLock sync.RWMutex
	writeLock          WriteLock
	messageBus         chan *manager.MessageResponse
}

var (
	ErrMinecraftNotRunning     = fmt.Errorf("minecraft server isn't running")
	ErrMinecraftAlreadyRunning = fmt.Errorf("minecraft server is already running")
	ErrNoLockAcquired          = fmt.Errorf("no lock acquired")
)

type RPCHandler struct {
	clientCounter atomic.Uint64
	managerServer *ManagerServer
}

type RPCConnInfo string

func (h *RPCHandler) TagConn(ctx context.Context, a *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, RPCConnInfo("id"), h.clientCounter.Add(1))
}

func (h *RPCHandler) HandleConn(c context.Context, s stats.ConnStats) {
	switch s.(type) {
	case *stats.ConnEnd:
		clientId := c.Value(RPCConnInfo("id")).(uint64)
		Println(color.RedString("客户端 Id:"), color.GreenString("%d ", clientId), color.RedString("断开连接"))
		h.managerServer.writeLock.Unlock(&manager.Client{Id: clientId})
	}
}

func (h *RPCHandler) TagRPC(ctx context.Context, s *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *RPCHandler) HandleRPC(context.Context, stats.RPCStats) {
}

type MinecraftState string

func (ms *ManagerServer) logForwardWorker() {
	if ms.forwardWorker {
		return
	}
	ms.forwardWorker = true
	if ms.minecraftInstance.pty == nil {
		ms.forwardWorker = false
		return
	}
	scanner := bufio.NewScanner(ms.minecraftInstance.pty)
	scanner.Buffer(make([]byte, 1048576), 1048576)
	for scanner.Scan() {
		line := scanner.Text()
		ms.writeLock.clientLock.RLock()
		locked := ms.writeLock.lockedClient != nil
		ms.writeLock.clientLock.RUnlock()
		ms.forwardChannelLock.RLock()
		for _, target := range ms.forwardChannels {
			select {
			default:
				// 防止阻塞线程
				Println(color.YellowString("客户端["), color.GreenString("%d", target.id), color.YellowString("]"), color.RedString("日志被丢弃："), color.YellowString(line))
			case target.channel <- &ForwardChannelMessage{message: line, locked: locked}:
				// do nothing
			}
		}
		ms.forwardChannelLock.RUnlock()
	}
	err := scanner.Err()
	if err != nil {
		Println(color.RedString("scanner 意外关闭:%v", err))
	}
	ms.forwardWorker = false
}

type MinecraftPty struct {
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	Stderr    io.ReadCloser
	stdin     io.WriteCloser
	stdout    io.Reader
	stderr    io.Reader
	multiRead io.Reader
}

func (pty *MinecraftPty) Init() {
	pty.stdout = pty.readerWrapper(pty.Stdout)
	pty.stderr = pty.readerWrapper(pty.Stderr)
	pty.stdin = pty.writerWrapper(pty.Stdin)
	if pty.multiRead == nil {
		pty.multiRead = io.MultiReader(pty.stdout, pty.stderr)
	}

}

func (pty *MinecraftPty) Read(p []byte) (int, error) {
	return pty.multiRead.Read(p)
}

func (pty *MinecraftPty) Write(p []byte) (int, error) {
	return pty.stdin.Write(p)
}

func (pty *MinecraftPty) Close() error {
	stdinerr := pty.Stdin.Close()
	if stdinerr != nil {
		return stdinerr
	}
	stdinerr = pty.stdin.Close()
	if stdinerr != nil {
		return stdinerr
	}
	stdouterr := pty.Stdout.Close()
	if stdouterr != nil {
		return stdouterr
	}
	stderrerr := pty.Stderr.Close()
	if stderrerr != nil {
		return stderrerr
	}
	return nil
}

func (ms *ManagerServer) RegisterForwardChannel() (channel *ForwardChannel) {
	ms.forwardChannelLock.Lock()
	defer ms.forwardChannelLock.Unlock()
	channel = &ForwardChannel{channel: make(chan *ForwardChannelMessage, 16384)}
	ms.forwardChannels = append(ms.forwardChannels, channel)
	return
}

func (ms *ManagerServer) UnregisterForwardChannel(channel *ForwardChannel) {
	ms.forwardChannelLock.Lock()
	defer ms.forwardChannelLock.Unlock()
	idx := slices.Index(ms.forwardChannels, channel)
	if idx >= 0 {
		ms.forwardChannels = slices.Delete(ms.forwardChannels, idx, idx+1)
	}
}

func (ms *ManagerServer) stopDetect() {
	if ms.minecraftInstance.process != nil {
		ms.minecraftInstance.process.Process.Wait()
		ms.minecraftInstance.pty.Close()
		ms.minecraftInstance.state = manager.MinecraftState_stopped
		ms.messageBus <- &manager.MessageResponse{Type: "StateChange", Content: "GameServerStop"}
		Println(color.RedString("服务器关闭"))
	}
}

func (ms *ManagerServer) Start(ctx context.Context, req *manager.StartRequest) (c *manager.StatusResponse, err error) {
	if ms.minecraftInstance.state != manager.MinecraftState_stopped {
		return nil, ErrMinecraftAlreadyRunning
	}
	ms.minecraftInstance.state = manager.MinecraftState_running
	Println(color.YellowString("客户端["), color.GreenString("%d", req.Client.Id), color.YellowString("]: 启动服务器: "), color.MagentaString(req.Path))
	cmd := exec.Command(filepath.Clean(req.Path))

	cmd.Dir = filepath.Dir(filepath.Clean(req.Path))
	cmd.SysProcAttr = MinecraftProcess_SysProcAttr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	mcpty := &MinecraftPty{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	mcpty.Init()
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	ms.minecraftInstance.process = cmd
	ms.minecraftInstance.pty = mcpty
	ms.messageBus <- &manager.MessageResponse{Type: "StateChange", Content: "StartGameServer"}
	if !ms.forwardWorker {
		go ms.logForwardWorker()
	}
	go ms.stopDetect()
	return &manager.StatusResponse{
		State: ms.minecraftInstance.state,
	}, nil
}

func (ms *ManagerServer) Message(client *manager.Client, server manager.Manager_MessageServer) error {
	Println(color.YellowString("接受客户端["), color.GreenString("%d", client.Id), color.YellowString("]的消息流监听请求"))
	message := ms.RegisterForwardChannel()
forward:
	for {
		select {
		case msg, ok := <-message.channel:
			if !ok {
				break forward
			}
			server.Send(&manager.MessageResponse{Id: message.id, Type: "stdout", Content: msg.message, Locked: msg.locked})
			message.id++
		case msg := <-ms.messageBus:
			msg.Id = message.id
			server.Send(msg)
			message.id++
		case <-server.Context().Done():
			Println(color.RedString("取消注册客户端["), color.GreenString("%d", client.Id), color.RedString("]的消息流监听请求"))
			ms.UnregisterForwardChannel(message)
			break forward
		}
	}
	return nil
}

func (ms *ManagerServer) Lock(ctx context.Context, client *manager.Client) (e *emptypb.Empty, err error) {
	ms.writeLock.Lock(client, false)
	select {
	case <-ctx.Done():
		Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]已离开排队队列，释放锁"))
		ms.writeLock.Unlock(client)
		return nil, nil
	default:
	}
	return nil, nil
}
func (ms *ManagerServer) Unlock(ctx context.Context, client *manager.Client) (e *emptypb.Empty, err error) {
	Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]主动释放锁"))
	ms.writeLock.Unlock(client)
	return nil, nil
}

func (ms *ManagerServer) Write(ctx context.Context, req *manager.WriteRequest) (e *emptypb.Empty, err error) {
	ms.writeLock.clientLock.RLock()
	lockedClient := ms.writeLock.lockedClient
	ms.writeLock.clientLock.RUnlock()
	if lockedClient == nil || req.Client == nil {
		return nil, ErrNoLockAcquired
	}
	if lockedClient.Id != req.Client.Id {
		return nil, ErrNoLockAcquired
	}
	if ms.minecraftInstance.state != manager.MinecraftState_running {
		return nil, ErrMinecraftNotRunning
	}
	ms.writeLock.Lock(req.Client, true)
	Println(color.YellowString("客户端["), color.GreenString("%d", req.Client.Id), color.YellowString("]向控制台写入[Seq: "), color.GreenString("%d", req.Id), color.YellowString("]: "), color.CyanString(req.Content))
	ms.minecraftInstance.pty.Write([]byte(req.Content + "\n"))
	return nil, nil
}

func (ms *ManagerServer) Login(ctx context.Context, req *emptypb.Empty) (c *manager.Client, err error) {
	c = &manager.Client{
		Id: ctx.Value(RPCConnInfo("id")).(uint64),
	}
	Println(color.YellowString("接受新客户端链接，分配 Id:%s", color.GreenString("%d", c.Id)))
	return
}

func (ms *ManagerServer) Status(ctx context.Context, client *manager.Client) (c *manager.StatusResponse, err error) {
	if ms.minecraftInstance.process == nil || ms.minecraftInstance.process.Process == nil {
		return &manager.StatusResponse{
			State: manager.MinecraftState_stopped,
		}, nil
	}
	MinecraftProcess, err := process.NewProcess(int32(ms.minecraftInstance.process.Process.Pid))
	Usedmemory := uint64(0)
	if err == nil {
		memoryInfo, err := MinecraftProcess.MemoryInfo()
		if err == nil {
			Usedmemory += memoryInfo.RSS
		}
		children, err := MinecraftProcess.Children()
		if err == nil {
			for _, p := range children {
				memoryInfo, err = p.MemoryInfo()
				if err == nil {
					Usedmemory += memoryInfo.RSS
				}
			}
		}
	}
	return &manager.StatusResponse{
		State:      ms.minecraftInstance.state,
		Usedmemory: Usedmemory,
	}, nil
}

func (ms *ManagerServer) printLogWorker() {
	message := ms.RegisterForwardChannel()

	go func() {
		for {
			msg, ok := <-message.channel
			if !ok {
				break
			}
			ms.writeLock.clientLock.RLock()
			lockedClient := ms.writeLock.lockedClient
			ms.writeLock.clientLock.RUnlock()
			if lockedClient != nil {
				Println(color.YellowString("服务器日志[Locked Client: "), color.GreenString("%d", lockedClient.Id), color.YellowString("]: "), color.CyanString(msg.message))
			}
		}
	}()
}

func (ms *ManagerServer) Stop(ctx context.Context, client *manager.Client) (c *emptypb.Empty, err error) {
	Println(color.YellowString("客户端["), color.GreenString("%d", client.Id), color.YellowString("]请求关闭服务器"))
	message := ms.RegisterForwardChannel()

	go func() {
		for {
			msg, ok := <-message.channel
			if !ok {
				break
			}
			Println(color.YellowString("服务器日志: "), color.CyanString(msg.message))
		}
	}()
	if ms.minecraftInstance.state == manager.MinecraftState_running {
		ms.minecraftInstance.pty.Write([]byte("stop\n"))
		time.AfterFunc(10*time.Second, func() {
			err := ms.minecraftInstance.process.Process.Signal(syscall.SIGTERM)
			Println(color.RedString("服务器关闭超时，发送 SIGTERM 信号 err:"), color.GreenString("%v", err))
		})
		ms.minecraftInstance.process.Process.Wait()
		ms.minecraftInstance.pty.Close()
	}
	ms.UnregisterForwardChannel(message)
	return nil, nil
}

func NewManagerServer() (m *ManagerServer) {
	m = &ManagerServer{
		messageBus: make(chan *manager.MessageResponse, 32),
	}
	m.printLogWorker()
	return m
}

func main() {

	managerServer := NewManagerServer()
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 12345))
	if err != nil {
		os.Exit(1)
	}
	rpcServer := grpc.NewServer(grpc.StatsHandler(&RPCHandler{managerServer: managerServer}))
	manager.RegisterManagerServer(rpcServer, managerServer)
	go func() {
		rpcServer.Serve(listener)
	}()
	sysSignals := make(chan os.Signal, 1)
	signal.Notify(sysSignals, syscall.SIGINT, syscall.SIGTERM)
	Println(color.YellowString("GameManager已启动, 等待客户端链接"))
	for {
		<-sysSignals
		Println(color.RedString("接受到 SIGTERM/SIGINT 信号，正在关闭服务器"))

		if managerServer.minecraftInstance.process != nil {
			managerServer.Stop(context.Background(), &manager.Client{
				Id: 0,
			})
		}
		Println(color.RedString("GameManager已关闭"))
		os.Exit(0)
	}
}
