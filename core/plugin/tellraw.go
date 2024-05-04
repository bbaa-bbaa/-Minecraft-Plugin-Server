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

package plugin

import (
	"encoding/json"
	"fmt"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
)

type TellrawManager struct {
	BasePlugin
}

func (tm *TellrawManager) DisplayName() string {
	return "命令回显"
}

func (tm *TellrawManager) Name() string {
	return "TellrawManager"
}

func (tm *TellrawManager) Init(pm pluginabi.PluginManager) (err error) {
	err = tm.BasePlugin.Init(pm, tm)
	if err != nil {
		return err
	}
	return nil
}

func (tm *TellrawManager) clickTriggerWrapper(p pluginabi.PluginName, msg []tellraw.Message) (out []tellraw.Message) {
	for _, m := range msg {
		if m.ClickEvent != nil && m.ClickEvent.Action == tellraw.RunCommand && m.ClickEvent.GoFunc != nil {
			m.ClickEvent.Value = fmt.Sprintf("/trigger %s", tm.scoreboardCore.registerTrigger(p, m.ClickEvent.GoFunc))
		}
		out = append(out, m)
	}
	return out
}

func (tm *TellrawManager) Tellraw(p pluginabi.PluginName, Target string, msg []tellraw.Message) {
	msg = append([]tellraw.Message{
		{Text: "[", Color: tellraw.Yellow, Bold: true},
		{Text: p.DisplayName(), Color: tellraw.Green, Bold: true},
		{Text: "] ", Color: tellraw.Yellow, Bold: true},
	}, msg...)
	msg = tm.clickTriggerWrapper(p, msg)
	jsonMsg, _ := json.Marshal(msg)
	tm.RunCommand(fmt.Sprintf("tellraw %s %s", Target, jsonMsg))
}
