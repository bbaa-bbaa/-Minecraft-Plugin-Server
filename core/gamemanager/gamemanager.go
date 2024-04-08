package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	pb "cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"github.com/shirou/gopsutil/v3/process"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MinecraftVistor struct {
	process *exec.Cmd
	pty     io.ReadWriteCloser
	state   pb.MinecraftState
}

type WriteLock struct {
	mutex        sync.Mutex
	lockedClient *pb.Client
	time         *time.Timer
}

const lockMaxTime = 10 * time.Second

func (wl *WriteLock) Unlock(client *pb.Client) {
	if wl.lockedClient != nil && wl.lockedClient.Id == client.Id {
		if wl.time != nil {
			wl.time.Stop()
			wl.time = nil
		}
		wl.lockedClient = nil
		wl.mutex.Unlock()
	}
}

func (wl *WriteLock) Lock(client *pb.Client) {
	if wl.lockedClient != nil && client.Id == wl.lockedClient.Id {
		wl.time.Reset(lockMaxTime)
		return
	}
	wl.mutex.Lock()
	wl.lockedClient = client
	wl.time = time.AfterFunc(lockMaxTime, func() {
		fmt.Printf("客户端[%d]的写入锁因超时而被取消\n", client.Id)
		wl.Unlock(client)
	})
}

type ManagerServer struct {
	pb.UnimplementedManagerServer

	minecraftInstance MinecraftVistor
	forwardWorker     bool
	forwardChannels   []chan string
	writeLock         WriteLock
}

var (
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
		fmt.Printf("客户端 Id:%d 断开连接\n", clientId)
		h.managerServer.writeLock.Unlock(&pb.Client{Id: clientId})
	}
}

func (h *RPCHandler) TagRPC(ctx context.Context, s *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *RPCHandler) HandleRPC(context.Context, stats.RPCStats) {
}

var manager *ManagerServer

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
	for scanner.Scan() {
		line := scanner.Text()
		for _, target := range ms.forwardChannels {
			select {
			default:
				// 防止阻塞线程
			//	fmt.Println("日志被丢弃!!:" + line)
			case target <- line:
				// do nothing
			}
		}
	}
}

type MinecraftPty struct {
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	multiRead io.Reader
}

func (pty *MinecraftPty) Read(p []byte) (int, error) {
	if pty.multiRead == nil {
		pty.multiRead = io.MultiReader(pty.stdout, pty.stderr)
	}
	return pty.multiRead.Read(p)
}

func (pty *MinecraftPty) Write(p []byte) (int, error) {
	return pty.stdin.Write(p)
}

func (pty *MinecraftPty) Close() error {
	stdinerr := pty.stdin.Close()
	if stdinerr != nil {
		return stdinerr
	}
	stdouterr := pty.stdout.Close()
	if stdouterr != nil {
		return stdouterr
	}
	stderrerr := pty.stderr.Close()
	if stderrerr != nil {
		return stderrerr
	}
	return nil
}

func (ms *ManagerServer) Start(ctx context.Context, req *pb.StartRequest) (c *pb.StatusResponse, err error) {
	if ms.minecraftInstance.process != nil && !ms.minecraftInstance.process.ProcessState.Exited() {
		return nil, ErrMinecraftAlreadyRunning
	}
	fmt.Printf("客户端[%d]: 启动服务器:%s\n", req.Client.Id, req.Path)
	cmd := exec.Command(req.Path)
	ms.minecraftInstance.process = cmd
	cmd.Dir = path.Dir(req.Path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
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
	mcpty := &MinecraftPty{stdin: stdin, stdout: stdout, stderr: stderr}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	ms.minecraftInstance.pty = mcpty
	ms.minecraftInstance.state = pb.MinecraftState_running
	if !ms.forwardWorker {
		go ms.logForwardWorker()
	}
	return &pb.StatusResponse{
		State: ms.minecraftInstance.state,
	}, nil
}

func (ms *ManagerServer) Message(client *pb.Client, server pb.Manager_MessageServer) error {
	fmt.Printf("接受客户端[%d]的消息流监听请求\n", client.Id)
	message := make(chan string, 16384)
	ms.forwardChannels = append(ms.forwardChannels, message)
	for {
		select {
		case msg := <-message:
			server.Send(&pb.MessageResponse{Id: 0, Type: "stdout", Content: msg})
		case <-server.Context().Done():
			idx := slices.Index(ms.forwardChannels, message)
			if idx >= 0 {
				ms.forwardChannels = slices.Delete(ms.forwardChannels, idx, idx)
			}
			fmt.Printf("取消注册客户端[%d]的消息流监听请求\n", client.Id)
			return nil
		}
	}
}

func (ms *ManagerServer) Lock(ctx context.Context, client *pb.Client) (e *emptypb.Empty, err error) {
	ms.writeLock.Lock(client)
	select {
	case <-ctx.Done():
		fmt.Printf("客户端[%d]已离开排队队列，释放锁\n", client.Id)
		ms.writeLock.Unlock(client)
		return &emptypb.Empty{}, nil
	default:
	}
	return &emptypb.Empty{}, nil
}
func (ms *ManagerServer) Unlock(ctx context.Context, client *pb.Client) (e *emptypb.Empty, err error) {
	ms.writeLock.Unlock(client)
	return &emptypb.Empty{}, nil
}

func (ms *ManagerServer) Write(ctx context.Context, req *pb.WriteRequest) (e *emptypb.Empty, err error) {
	if ms.writeLock.lockedClient == nil || req.Client == nil {
		return nil, ErrNoLockAcquired
	}
	if ms.writeLock.lockedClient != nil && ms.writeLock.lockedClient.Id != req.Client.Id {
		return nil, ErrNoLockAcquired
	}
	ms.writeLock.Lock(req.Client)
	ms.minecraftInstance.pty.Write([]byte(req.Content + "\n"))
	return &emptypb.Empty{}, nil
}

func (ms *ManagerServer) Login(ctx context.Context, req *emptypb.Empty) (c *pb.Client, err error) {
	c = &pb.Client{
		Id: ctx.Value(RPCConnInfo("id")).(uint64),
	}
	fmt.Printf("接受新客户端链接，分配 Id:%d\n", c.Id)
	return
}

func (ms *ManagerServer) Status(ctx context.Context, client *pb.Client) (c *pb.StatusResponse, err error) {
	if ms.minecraftInstance.process == nil {
		return &pb.StatusResponse{
			State: pb.MinecraftState_stopped,
		}, nil
	}
	MinecraftProcess, err := process.NewProcess(int32(ms.minecraftInstance.process.Process.Pid))
	Usedmemory := uint64(0)
	if err == nil {
		memoryInfo, err := MinecraftProcess.MemoryInfo()
		if err == nil {
			Usedmemory = memoryInfo.RSS
		}
	}
	return &pb.StatusResponse{
		State:      ms.minecraftInstance.state,
		Usedmemory: Usedmemory,
	}, nil
}

func main() {
	manager = &ManagerServer{}
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 12345))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	rpcServer := grpc.NewServer(grpc.StatsHandler(&RPCHandler{managerServer: manager}))
	pb.RegisterManagerServer(rpcServer, manager)
	go func() {
		rpcServer.Serve(listener)
	}()
	sysSignals := make(chan os.Signal, 1)
	signal.Notify(sysSignals, syscall.SIGINT, syscall.SIGTERM)
	for {
		<-sysSignals
		fmt.Println("接受到 SIGTERM/SIGINT 信号，正在关闭服务器")
		message := make(chan string, 1024)
		manager.forwardChannels = append(manager.forwardChannels, message)

		go func() {
			for {
				msg, ok := <-message
				if !ok {
					break
				}
				fmt.Printf("服务器日志： %s\n", msg)
			}
		}()

		if manager.minecraftInstance.process != nil {
			manager.minecraftInstance.pty.Write([]byte("stop\n"))
		}

		manager.minecraftInstance.process.Process.Wait()
		manager.minecraftInstance.pty.Close()
		fmt.Println("服务器关闭")
		close(message)
		os.Exit(0)
	}
}
