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

package pluginabi

import (
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/manager"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Plugin interface {
	PluginName
	Init(PluginManager) error
	Start()
	Pause()
}

type PluginName interface {
	Name() string
	DisplayName() string
}

type PluginNameWrapper struct {
	PluginName        string
	PluginDisplayName string
}

func (p *PluginNameWrapper) Name() string {
	return p.PluginName
}

func (p *PluginNameWrapper) DisplayName() string {
	if p.PluginDisplayName == "" {
		return p.PluginName
	}
	return p.PluginDisplayName
}

type PluginManager interface {
	Printf(scope string, format string, a ...any) (n int, err error)
	Println(scope string, a ...any) (n int, err error)
	RegisterLogProcesser(context PluginName, process func(logmsg string, iscommandrespone bool)) (channel chan *manager.MessageResponse)
	RegisterServerMessageProcesser(skipRegister bool) (channel chan *manager.MessageResponse)
	RegisterPlugin(plugin Plugin) (err error)
	GetPlugin(pluginName string) Plugin
	UnregisterServerMessageProcesser(channel chan *manager.MessageResponse)

	RunCommand(cmd string) string

	Status(opts ...grpc.CallOption) (*manager.StatusResponse, error)
	Stop(opts ...grpc.CallOption) (*emptypb.Empty, error)
	StartMinecraft() (err error)
}
