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
	"os"
	"path/filepath"
	"sync"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/go-co-op/gocron/v2"
)

type BackupPlugin struct {
	plugin.BasePlugin
	Source     string // Minecraft world source dir
	Dest       string // backup dest
	backupLock sync.Mutex
	cron       gocron.Scheduler
}

func (bp *BackupPlugin) DisplayName() string {
	return "简单备份"
}

func (bp *BackupPlugin) Name() string {
	return "BackupPlugin"
}

func (bp *BackupPlugin) SaveSize(src string) (int64, error) {
	var size int64
	err := filepath.Walk(src, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func (bp *BackupPlugin) MakePlayerDataBackup() {

}

func (bp *BackupPlugin) MakeBackup(comment string) {
	now := time.Now()
	dest := filepath.Join(bp.Dest, "world", comment+"_"+now.Format("2006_01_02_15_04_05"))
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		bp.TellrawError("@a", err)
	}
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "=== ", Color: tellraw.Yellow},
		{Text: "整世界备份", Color: tellraw.Light_Purple},
		{Text: " 时间：", Color: tellraw.Green},
		{Text: now.Format(time.RFC3339), Color: tellraw.Aqua, Bold: true},
		{Text: " ===", Color: tellraw.Yellow},
	})
	if !bp.backupLock.TryLock() {
		bp.Tellraw("@a", []tellraw.Message{{Text: "已有正在进行的备份进程", Color: tellraw.Yellow}})
		bp.Tellraw("@a", []tellraw.Message{{Text: "本次备份操作取消", Color: tellraw.Red}})
		return
	}
	defer bp.backupLock.Unlock()
	stat, err := os.Stat(bp.Source)
	if err != nil {
		bp.TellrawError("@a", err)
		return
	}
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "备注: ", Color: tellraw.Yellow},
		{Text: comment, Color: tellraw.Green},
	})
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "保存时间: ", Color: tellraw.Yellow},
		{Text: stat.ModTime().Format(time.RFC3339), Color: tellraw.Green},
	})
	size, err := bp.SaveSize(bp.Source)
	if err != nil {
		bp.TellrawError("@a", err)
	}
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "存档大小: ", Color: tellraw.Yellow},
		{Text: fmt.Sprintf("%.2fMiB", float64(size)/1024/1024), Color: tellraw.Green},
	})
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "正在复制存档", Color: tellraw.Red},
	})
	err = bp.Copy(bp.Source, dest)
	if err != nil {
		bp.TellrawError("@a", err)
		return
	}
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "备份完成", Color: tellraw.Green},
	})
	bp.Tellraw("@a", []tellraw.Message{
		{Text: "<<< ", Color: tellraw.Aqua},
		{Text: "整世界备份", Color: tellraw.Light_Purple},
		{Text: " 时间：", Color: tellraw.Green},
		{Text: now.Format(time.RFC3339), Color: tellraw.Aqua, Bold: true},
		{Text: " >>>", Color: tellraw.Aqua},
	})
}

func (bp *BackupPlugin) Init(pm pluginabi.PluginManager) (err error) {
	bp.cron, _ = gocron.NewScheduler()
	bp.cron.NewJob(gocron.CronJob("*/30 * * * *", false), gocron.NewTask(func() {
		bp.MakeBackup("AutoBackup")
	}), gocron.WithSingletonMode(gocron.LimitModeReschedule))
	err = bp.BasePlugin.Init(pm, bp)
	if err != nil {
		return err
	}
	bp.RegisterCommand("backup", func(s1 string, s2 ...string) {
		if len(s2) != 2 {
			bp.Tellraw(s1, []tellraw.Message{{Text: "备份备注不能为空或含有空格", Color: tellraw.Red}})
			return
		}
		if s2[0] == "backup" {
			bp.MakeBackup(s2[1])
		}
	})
	return nil
}

func (bp *BackupPlugin) Start() {
	bp.cron.Start()
}

func (bp *BackupPlugin) Pause() {
	bp.cron.StopJobs()
}
