package core

import (
	"context"
	"fmt"
	"strings"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"github.com/fatih/color"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GameManagerMessageBus struct {
	client   manager.Manager_MessageClient
	channels []chan *manager.MessageResponse
}

type MinecraftManagerClient struct {
	Address          string
	StartScript      string
	ClientInfo       *manager.Client
	client           manager.ManagerClient
	context          context.Context
	messageBus       GameManagerMessageBus
	commandProcessor *MinecraftCommandProcessor
}

func (mmc *MinecraftManagerClient) RunCommand(cmd string) string {
	return mmc.commandProcessor.RunCommand(cmd)
}
func (mmc *MinecraftManagerClient) Lock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mmc.client.Lock(mmc.context, mmc.ClientInfo, opts...)
}
func (mmc *MinecraftManagerClient) Unlock(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mmc.client.Unlock(mmc.context, mmc.ClientInfo, opts...)
}
func (mmc *MinecraftManagerClient) Write(wr *manager.WriteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	wr.Client = mmc.ClientInfo
	return mmc.client.Write(mmc.context, wr, opts...)
}
func (mmc *MinecraftManagerClient) Start(st *manager.StartRequest, opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	st.Client = mmc.ClientInfo
	return mmc.client.Start(mmc.context, st, opts...)
}
func (mmc *MinecraftManagerClient) Stop(opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return mmc.client.Stop(mmc.context, mmc.ClientInfo, opts...)
}
func (mmc *MinecraftManagerClient) Status(opts ...grpc.CallOption) (*manager.StatusResponse, error) {
	return mmc.client.Status(mmc.context, mmc.ClientInfo, opts...)
}

func (mmc *MinecraftManagerClient) Printf(scope string, format string, a ...any) (n int, err error) {

	return fmt.Printf(color.YellowString("[")+"%s"+color.YellowString("] ")+strings.TrimRight(format, "\r\n")+"\r\n", append([]any{scope}, a...)...)
}

func (mmc *MinecraftManagerClient) Println(scope string, a ...any) (n int, err error) {
	return mmc.Printf(scope, "%s", strings.TrimRight(fmt.Sprint(a...), "\r\n"))
}

func (mmc *MinecraftManagerClient) kPrintln(a ...any) (n int, err error) {
	return mmc.Println(color.MagentaString("MinecraftManager"), a...)
}

func (mmc *MinecraftManagerClient) login() (err error) {
	mmc.ClientInfo, err = mmc.client.Login(mmc.context, nil)
	if err != nil {
		mmc.kPrintln(color.RedString("获取 Client ID 失败: " + err.Error()))
		return err
	}
	mmc.kPrintln(color.YellowString("从 GameManager 获取 ClientId:%s ", color.GreenString("%d", mmc.ClientInfo.Id)))
	return nil
}

func (mmc *MinecraftManagerClient) messageForwardWorker() {
	mmc.kPrintln(color.YellowString("消息转发 Worker 启动"))
	for {
		message, err := mmc.messageBus.client.Recv()
		if err != nil {
			mmc.kPrintln(color.RedString("MessageBus 关闭"))
			break
		}
		for _, channel := range mmc.messageBus.channels {
			select {
			case channel <- message:
			default:
			}
		}
		go func() {
			if strings.Contains(message.Content, "abcd") {
				mmc.RunCommand("tellraw @a {\"text\":\"1234\"}")
				fmt.Println(mmc.RunCommand("list"))
			}
		}()

	}
}

func (mmc *MinecraftManagerClient) registerLogProcesser(process func(string)) {
	channel := make(chan *manager.MessageResponse, 16384)
	mmc.messageBus.channels = append(mmc.messageBus.channels, channel)
	go func() {
		for msg := range channel {
			switch msg.Type {
			case "stdout":
				process(msg.Content)
			}
		}
	}()

}

func (mmc *MinecraftManagerClient) registerServerMessageListener() (err error) {
	mmc.messageBus.client, err = mmc.client.Message(mmc.context, mmc.ClientInfo)
	if err != nil {
		mmc.kPrintln(color.RedString("无法注册服务消息侦听器，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	go mmc.messageForwardWorker()
	return nil
}

func (mmc *MinecraftManagerClient) getStatus() (status *manager.StatusResponse, err error) {
	status, err = mmc.Status()
	if err != nil {
		mmc.kPrintln(color.RedString("无法获取服务器在状态: %s", err.Error()))
		return nil, err
	}
	return status, nil
}

func (mmc *MinecraftManagerClient) startMinecraft() (err error) {
	_, err = mmc.Start(&manager.StartRequest{Client: mmc.ClientInfo, Path: mmc.StartScript})
	if err != nil {
		mmc.kPrintln(color.RedString("Minecraft 服务器启动失败: %s", err.Error()))
		return err
	}
	return nil
}

func (mmc *MinecraftManagerClient) initClient() (err error) {
	mmc.kPrintln(color.YellowString("正在登录 Manager Backend"))
	err = mmc.login()
	if err != nil {
		return err
	}
	mmc.kPrintln(color.YellowString("正在注册 MessageBus/Stdout 转发器"))
	err = mmc.registerServerMessageListener()
	if err != nil {
		return err
	}
	mmc.kPrintln(color.YellowString("正在注册命令处理器"))
	mmc.commandProcessor = NewCommandProcessor(mmc)
	mmc.kPrintln(color.YellowString("正在获取服务器状态"))
	status, err := mmc.getStatus()
	if err != nil {
		return err
	}
	switch status.State {
	case manager.MinecraftState_running:
		mmc.kPrintln(color.GreenString("Minecraft 正在运行"))
	case manager.MinecraftState_stopped:
		mmc.kPrintln(color.YellowString("正在启动 Minecraft 服务器"))
		err = mmc.startMinecraft()
		if err != nil {
			return err
		}
		mmc.kPrintln(color.GreenString("Minecraft 启动请求发送"))
	}
	return nil
}

func (mmc *MinecraftManagerClient) Dial(server string) (err error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	mmc.Address = server
	conn, err := grpc.Dial(mmc.Address, opts...)
	if err != nil {
		mmc.kPrintln(color.RedString("无法连接上 Manager Backend，请检查 Backend 是否运行: %s", err.Error()))
		return err
	}
	mmc.client = manager.NewManagerClient(conn)
	mmc.context = context.Background()

	return mmc.initClient()
}
