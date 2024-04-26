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
	"fmt"
	"strings"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"github.com/samber/lo"
)

type MinecraftPosition struct {
	Position  [3]float64
	Dimension string
}

type TeleportCore struct {
	BasePlugin
}

func (tc *TeleportCore) Init(pm pluginabi.PluginManager) (err error) {
	err = tc.BasePlugin.Init(pm, tc)
	if err != nil {
		return err
	}
	return nil
}

func (tc *TeleportCore) TeleportPlayer(src string, dst string) error {
	// 先尝试直接 tp
	res := tc.RunCommand(fmt.Sprintf(`tp %s %s`, src, dst))
	if strings.Contains(res, "No entity") {
		return fmt.Errorf("目标玩家不存在")
	}
	if !strings.Contains(res, "Teleported") {
		// 跨世界 TP
		dstPi, err := tc.GetPlayerInfo_Position(dst)
		if err != nil {
			return fmt.Errorf("无法获取目标玩家信息")
		}
		tc.RunCommand(fmt.Sprintf(`execute as %s rotated as %s in %s run tp %s`, src, dst, dstPi.LastLocation.Dimension, dst))
	}
	return nil
}

func (tc *TeleportCore) TeleportPosition(src string, dst *MinecraftPosition) error {
	tc.RunCommand(fmt.Sprintf(`execute as %s rotated as %s in %s run tp %s`, src, src, dst.Dimension, strings.Join(lo.Map(dst.Position[:], func(item float64, index int) string {
		return fmt.Sprintf("%f", item)
	}), " ")))
	return nil
}

func (tc *TeleportCore) Teleport(src string, dst any) error {
	// 存储目前位置
	pi, err := tc.GetPlayerInfo_Position(src)
	if err != nil {
		return err
	}
	pi.LastLocation = pi.Location
	pi.Commit()
	switch dst := dst.(type) {
	case string:
		return tc.TeleportPlayer(src, dst)
	case *MinecraftPosition:
		return tc.TeleportPosition(src, dst)
	}
	return nil
}

func (tc *TeleportCore) Name() string {
	return "TeleportCore"
}

func (tc *TeleportCore) DisplayName() string {
	return "传送内核"
}
