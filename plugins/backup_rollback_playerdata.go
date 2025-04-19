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

package plugins

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/fatih/color"
)

type RollbackPlayerdataPending struct {
	player    string
	pi        *plugin.MinecraftPlayerInfo
	cancel    *time.Timer
	comfirm   *time.Ticker
	countdown int
	name      string
	path      string
	bp        *BackupPlugin
	fstat     fs.FileInfo
	lock      sync.Mutex
}

func (rpp *RollbackPlayerdataPending) Start(caller *BackupPlugin) {
	var err error
	rpp.bp = caller
	rpp.pi, err = rpp.bp.GetPlayerInfo(rpp.player)
	if err != nil {
		rpp.bp.TellrawError("@a", err)
		return
	}
	rpp.path = filepath.Join(rpp.bp.Dest, "playerdata", rpp.pi.UUID, rpp.name)
	rpp.fstat, err = os.Stat(rpp.path)
	if err != nil {
		rpp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "找不到所请求的备份文件", Color: tellraw.Red},
		})
		return
	}
	rpp.bp.rollbackLock.RLock()
	pd := rpp.bp.rollbackPending
	rpp.bp.rollbackLock.RUnlock()
	if pd != nil {
		rpp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "已有正在进行的回档请求", Color: tellraw.Red},
		})
		return
	}
	rpp.bp.rollbackLock.Lock()
	rpp.bp.rollbackPending = rpp
	rpp.bp.rollbackLock.Unlock()
	rpp.cancel = time.AfterFunc(10*time.Second, func() {
		rpp.Abort(rpp.player)
	})
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "======== ", Color: tellraw.Red},
		{Text: "玩家数据回档请求确认", Color: tellraw.Light_Purple},
		{Text: " ========", Color: tellraw.Red},
	})
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "玩家: ", Color: tellraw.Yellow},
		{Text: rpp.player, Color: tellraw.Green},
	})
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "名称: ", Color: tellraw.Yellow},
		{Text: rpp.name, Color: tellraw.Green},
	})
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "时间: ", Color: tellraw.Yellow},
		{Text: rpp.fstat.ModTime().Format(time.RFC3339), Color: tellraw.Green},
	})
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "输入[", Color: tellraw.Yellow},
		{Text: "!!backup confirm", Color: tellraw.Red,
			ClickEvent: &tellraw.ClickEvent{
				Action: tellraw.SuggestCommand,
				Value:  "!!backup confirm",
			},
		},
		{Text: "]继续 ", Color: tellraw.Yellow},
		{Text: "点击[", Color: tellraw.Yellow},
		{Text: "!!backup cancel", Color: tellraw.Red,
			ClickEvent: &tellraw.ClickEvent{
				Action: tellraw.RunCommand,
				GoFunc: func(s string, i int) {
					rpp.Abort(rpp.player)
				},
			},
		},
		{Text: "]取消", Color: tellraw.Yellow},
	})
}

func (rpp *RollbackPlayerdataPending) Execute() {
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: fmt.Sprintf("%d", rpp.countdown), Color: tellraw.Yellow},
		{Text: " 秒后将回档玩家 ", Color: tellraw.Red},
		{Text: rpp.player, Color: tellraw.Yellow},
		{Text: " 数据", Color: tellraw.Red},
	})
	for range rpp.comfirm.C {
		rpp.countdown--
		rpp.bp.Tellraw("@a", []tellraw.Message{
			{Text: fmt.Sprintf("%d", rpp.countdown), Color: tellraw.Yellow},
			{Text: " 秒后将回档玩家 ", Color: tellraw.Red},
			{Text: rpp.player, Color: tellraw.Yellow},
			{Text: " 数据", Color: tellraw.Red},
		})
		if rpp.countdown == 0 {
			break
		}
	}
	rpp.comfirm.Stop()
	rpp.bp.Println(color.RedString("回档玩家数据："), color.YellowString(rpp.path))
	rpp.bp.Println(color.YellowString("踢出玩家"))
	rpp.bp.RunCommand(fmt.Sprintf("kick %s 正在准备回档", rpp.player))
	rpp.bp.Println(color.YellowString("封禁玩家"))
	rpp.bp.RunCommand(fmt.Sprintf("ban %s 回档正在进行中", rpp.player))
	time.Sleep(2 * time.Second)
	rpp.bp.Println(color.RedString("复制玩家数据："), color.YellowString(rpp.path))
	rpp.bp.Copy(rpp.path, rpp.bp.Source)
	rpp.bp.Println(color.YellowString("解除封禁玩家"))
	rpp.bp.RunCommand(fmt.Sprintf("pardon %s", rpp.player))
	rpp.bp.rollbackLock.Lock()
	rpp.bp.rollbackPending = nil
	rpp.bp.rollbackLock.Unlock()
	rpp.lock.Unlock()
	rpp.bp.Println(color.GreenString("回档流程结束"))
}

func (rpp *RollbackPlayerdataPending) Comfirm(player string) {
	if player != rpp.player {
		rpp.bp.Tellraw(player, []tellraw.Message{
			{Text: "该请求只能由发起请求的玩家确认", Color: tellraw.Red},
		})
		return
	}
	if !rpp.lock.TryLock() {
		rpp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "请勿多次执行", Color: tellraw.Red},
		})
		return
	}
	rpp.cancel.Stop()
	rpp.countdown = 5
	rpp.comfirm = time.NewTicker(1 * time.Second)
	go rpp.Execute()
}

func (rpp *RollbackPlayerdataPending) Abort(player string) {
	rpp.bp.rollbackLock.Lock()
	rpp.bp.rollbackPending = nil
	rpp.bp.rollbackLock.Unlock()
	usercancel := rpp.cancel.Stop()
	if rpp.comfirm == nil && !usercancel {
		rpp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "回档请求超时", Color: tellraw.Red},
		})
	} else if rpp.comfirm != nil {
		rpp.comfirm.Stop()
	}
	rpp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "已取消本次回档请求", Color: tellraw.Red},
	})
}
