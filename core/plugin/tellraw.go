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

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
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

func (tm *TellrawManager) cleanUp(msg []tellraw.Message) (out []tellraw.Message) {
	for _, m := range msg {
		if (m.Type == "" || m.Type == tellraw.Text) && m.Text == "" {
			continue
		}
		if m.HoverEvent != nil && m.HoverEvent.Action == tellraw.Show_Text {
			if m.HoverEvent.Contents == nil {
				m.HoverEvent = nil
			} else {
				m.HoverEvent.Contents = tm.cleanUp(m.HoverEvent.Contents.([]tellraw.Message))
			}
		}
		out = append(out, m)
	}
	return out
}

func (tm *TellrawManager) clickTriggerWrapper(p pluginabi.PluginName, Selector string, msg []tellraw.Message) []tellraw.Message {
	triggerValueList := []*string{}
	triggerFuncList := []MinecraftTrigger{}
	for i := range msg {
		if msg[i].ClickEvent != nil && msg[i].ClickEvent.Action == tellraw.RunCommand && msg[i].ClickEvent.GoFunc != nil {
			triggerFuncList = append(triggerFuncList, MinecraftTrigger{Trigger: msg[i].ClickEvent.GoFunc, Time: msg[i].ClickEvent.TriggerTime, Selector: Selector})
			triggerValueList = append(triggerValueList, &msg[i].ClickEvent.Value)
		}
	}
	if len(triggerFuncList) > 0 {
		triggerName := tm.scoreboardCore.registerTrigger(p, triggerFuncList...)
		for idx, name := range triggerName {
			*triggerValueList[idx] = fmt.Sprintf(`/trigger %s`, name)
		}
	}
	return msg
}

func (tm *TellrawManager) Tellraw(p pluginabi.PluginName, Target string, msg []tellraw.Message) {
	msg = append([]tellraw.Message{
		{Text: "[", Color: tellraw.Yellow, Bold: true},
		{Text: p.DisplayName(), Color: tellraw.Green, Bold: true},
		{Text: "] ", Color: tellraw.Yellow, Bold: true},
	}, msg...)
	msg = tm.cleanUp(msg)
	msg = tm.clickTriggerWrapper(p, Target, msg)
	jsonMsg, _ := json.Marshal(msg)
	tm.RunCommand(fmt.Sprintf("tellraw %s %s", Target, jsonMsg))
}
