package pluginabi

import (
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Plugin interface {
	Init(PluginManager)
	Start()
	Pause()
	Name() string
}

type PluginName interface {
	Name() string
}

type PluginNameWrapper struct {
	PluginName string
}

func (p *PluginNameWrapper) Name() string {
	return p.PluginName
}

type PluginManager interface {
	Printf(scope string, format string, a ...any) (n int, err error)
	Println(scope string, a ...any) (n int, err error)
	RegisterLogProcesser(context PluginName, process func(string)) (channel chan *manager.MessageResponse)
	RegisterServerMessageProcesser() (channel chan *manager.MessageResponse)
	RegisterPlugin(plugin Plugin) (err error)
	GetPlugin(pluginName string) Plugin
	UnregisterServerMessageProcesser(channel chan *manager.MessageResponse)

	RunCommand(cmd string) string

	Status(opts ...grpc.CallOption) (*manager.StatusResponse, error)
	Stop(opts ...grpc.CallOption) (*emptypb.Empty, error)
	StartMinecraft() (err error)
}
