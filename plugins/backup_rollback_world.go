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

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/fatih/color"
)

type RollbackWorldPending struct {
	player    string
	cancel    *time.Timer
	comfirm   *time.Ticker
	countdown int
	name      string
	path      string
	bp        *BackupPlugin
	fstat     fs.FileInfo
	lock      sync.Mutex
}

func (rwp *RollbackWorldPending) Start(caller *BackupPlugin) {
	var err error
	rwp.bp = caller
	rwp.path = filepath.Join(rwp.bp.Dest, "world", rwp.name)
	rwp.fstat, err = os.Stat(rwp.path)
	if err != nil {
		rwp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "找不到所请求的备份文件", Color: tellraw.Red},
		})
		return
	}
	rwp.bp.rollbackLock.RLock()
	pd := rwp.bp.rollbackPending
	rwp.bp.rollbackLock.RUnlock()
	if pd != nil {
		rwp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "已有正在进行的回档请求", Color: tellraw.Red},
		})
		return
	}
	rwp.bp.rollbackLock.Lock()
	rwp.bp.rollbackPending = rwp
	rwp.bp.rollbackLock.Unlock()
	rwp.cancel = time.AfterFunc(10*time.Second, func() {
		rwp.Abort(rwp.player)
	})
	rwp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "======== ", Color: tellraw.Red},
		{Text: "回档请求确认", Color: tellraw.Light_Purple},
		{Text: " ========", Color: tellraw.Red},
	})
	rwp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "名称: ", Color: tellraw.Yellow},
		{Text: rwp.name, Color: tellraw.Green},
	})
	rwp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "时间: ", Color: tellraw.Yellow},
		{Text: rwp.fstat.ModTime().Format(time.RFC3339), Color: tellraw.Green},
	})
	rwp.bp.Tellraw("@a", []tellraw.Message{
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
					rwp.Abort(rwp.player)
				},
			},
		},
		{Text: "]取消", Color: tellraw.Yellow},
	})
}

func (rwp *RollbackWorldPending) Execute() {
	rwp.bp.Tellraw("@a", []tellraw.Message{
		{Text: fmt.Sprintf("%d", rwp.countdown), Color: tellraw.Yellow},
		{Text: " 秒后将重启服务器回档", Color: tellraw.Red},
	})
	for range rwp.comfirm.C {
		rwp.countdown--
		rwp.bp.Tellraw("@a", []tellraw.Message{
			{Text: fmt.Sprintf("%d", rwp.countdown), Color: tellraw.Yellow},
			{Text: " 秒后将重启服务器回档", Color: tellraw.Red},
		})
		if rwp.countdown == 0 {
			break
		}
	}
	rwp.comfirm.Stop()

	rwp.bp.backupLock.Lock()

	rwp.bp.Println(color.RedString("回档："), color.YellowString(rwp.path))
	rwp.bp.Println(color.RedString("关闭服务器"))
	rwp.bp.pm.Stop()
	rwp.bp.Println(color.RedString("释放存档"))
	os.RemoveAll(rwp.bp.Source)
	rwp.bp.Copy(rwp.path, rwp.bp.Source)
	rwp.bp.Println(color.YellowString("重启服务器"))

	rwp.bp.backupLock.Unlock()
	rwp.bp.pm.StartMinecraft()

	rwp.bp.rollbackLock.Lock()
	rwp.bp.rollbackPending = nil
	rwp.bp.rollbackLock.Unlock()
	rwp.lock.Unlock()
	rwp.bp.Println(color.GreenString("回档流程结束"))
}

func (rwp *RollbackWorldPending) Comfirm(player string) {
	if !rwp.lock.TryLock() {
		rwp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "请勿多次执行", Color: tellraw.Red},
		})
		return
	}
	rwp.cancel.Stop()
	rwp.countdown = 10
	rwp.comfirm = time.NewTicker(1 * time.Second)
	go rwp.Execute()
}

func (rwp *RollbackWorldPending) Abort(player string) {
	rwp.bp.rollbackLock.Lock()
	rwp.bp.rollbackPending = nil
	rwp.bp.rollbackLock.Unlock()
	usercancel := rwp.cancel.Stop()
	if rwp.comfirm == nil && !usercancel {
		rwp.bp.Tellraw("@a", []tellraw.Message{
			{Text: "回档请求超时", Color: tellraw.Red},
		})
	} else if rwp.comfirm != nil {
		rwp.comfirm.Stop()
	}
	rwp.bp.Tellraw("@a", []tellraw.Message{
		{Text: "已取消本次回档请求", Color: tellraw.Red},
	})
}
