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
	"slices"
	"strings"
	"sync"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"github.com/go-co-op/gocron/v2"
	"github.com/samber/lo"
)

const BackupPlugin_PageSize = 5

type BackupPlugin_RollbackPending interface {
	Comfirm(player string)
	Abort(player string)
	Start(caller *BackupPlugin)
}

type BackupPlugin struct {
	plugin.BasePlugin
	Source                 string // Minecraft world source dir
	Dest                   string // backup dest
	backupLock             sync.Mutex
	rollbackLock           sync.RWMutex
	cron                   gocron.Scheduler
	rollbackPending        BackupPlugin_RollbackPending
	backupPlayerdataTicker *time.Ticker
	pm                     pluginabi.PluginManager
	ExtPlayerdataDir       []string
	ExtPlayerdataExt       []string
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
	playerdataMtime := map[string]time.Time{}
	playerdataDir := append([]string{"playerdata", "advancements", "stats"}, bp.ExtPlayerdataDir...)
	dataExt := append([]string{".json", ".dat"}, bp.ExtPlayerdataExt...)
	backupUUID := []string{}
	for _, subdir := range playerdataDir {
		sd := filepath.SplitList(subdir)
		dir := filepath.Join(append([]string{bp.Source}, sd...)...)
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			uuid := strings.TrimSuffix(filepath.Base(path), ext)
			if slices.Contains(dataExt, ext) {
				fi, err := d.Info()
				mt := fi.ModTime()
				if err == nil {
					if playerdataMtime[uuid].IsZero() {
						playerdataMtime[uuid] = mt
					} else if playerdataMtime[uuid].Before(mt) {
						playerdataMtime[uuid] = mt
					}
				}
			}
			return nil
		})
	}
	for uuid, mtime := range playerdataMtime {
		var lastMtime time.Time
		pi, err := bp.GetPlayerInfo(uuid)
		if err != nil {
			continue
		}
		pi.GetExtra(bp, &lastMtime)
		if mtime.After(lastMtime) {
			backupUUID = append(backupUUID, uuid)
		}
		pi.PutExtra(bp, mtime)
	}
	for _, subdir := range playerdataDir {
		sd := filepath.SplitList(subdir)
		dir := filepath.Join(append([]string{bp.Source}, sd...)...)
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			uuid := strings.TrimSuffix(filepath.Base(path), ext)
			if !slices.Contains(backupUUID, uuid) {
				return nil
			}
			if slices.Contains(dataExt, ext) {
				dest := filepath.Join(bp.Dest, "playerdata", uuid, playerdataMtime[uuid].Format("2006_01_02_15_04_05"), subdir)
				os.MkdirAll(dest, 0755)
				bp.Copy(path, dest)
			}
			return nil
		})
	}
	for _, uuid := range backupUUID {
		mtime := playerdataMtime[uuid]
		dest := filepath.Join(bp.Dest, "playerdata", uuid, mtime.Format("2006_01_02_15_04_05"))
		os.Chtimes(dest, mtime, mtime)
	}
	bp.CleanupPlayerdataBackup()
}

func (bp *BackupPlugin) CleanupPlayerdataBackup() {
	playerDir, _ := os.ReadDir(filepath.Join(bp.Dest, "playerdata"))
	for _, playerSubDir := range playerDir {
		dir := filepath.Join(bp.Dest, "playerdata", playerSubDir.Name())
		backupFiles, _ := os.ReadDir(dir)
		backupList := bp.getBackupList(backupFiles)
		cleanList := backupList[min(len(backupList), 60):]
		for _, name := range cleanList {
			os.RemoveAll(filepath.Join(dir, name))
		}
	}
}

func (bp *BackupPlugin) CleanupBackup() {
	backupFiles, _ := os.ReadDir(filepath.Join(bp.Dest, "world"))
	backupList := bp.getBackupList(backupFiles)
	cleanList := backupList[min(len(backupList), 60):]
	for _, name := range cleanList {
		os.RemoveAll(filepath.Join(bp.Dest, "world", name))
	}
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
	bp.CleanupBackup()
}

func (bp *BackupPlugin) showList(list []string, start int, end int, execCmd func(string) tellraw.GoFunc, pager func(string, string)) {
	if start >= len(list) {
		bp.Tellraw("@a", []tellraw.Message{{Text: "该页没有内容", Color: tellraw.Red}})
	}
	start = min(max(0, start), len(list)-1)
	end = max(min(len(list), end), 0)
	listSlice := list[start:end]
	message := []tellraw.Message{
		{Text: "正在查看第", Color: tellraw.Aqua},
		{Text: fmt.Sprintf("%d", start/BackupPlugin_PageSize+1), Color: tellraw.Light_Purple},
		{Text: "页/", Color: tellraw.Aqua},
		{Text: "共", Color: tellraw.Aqua},
		{
			Text:  fmt.Sprintf("%d", (len(list)+BackupPlugin_PageSize-1)/BackupPlugin_PageSize),
			Color: tellraw.Light_Purple,
		},
		{Text: "页\n", Color: tellraw.Aqua},
	}
	for index, item := range listSlice {
		message = append(message, []tellraw.Message{
			{Text: fmt.Sprintf("%d.", index+1), Color: tellraw.Aqua},
			{Text: item, Color: tellraw.Yellow},
			{Text: "【点我选择】\n", Color: tellraw.Green, ClickEvent: &tellraw.ClickEvent{Action: tellraw.RunCommand, GoFunc: execCmd(item)}},
		}...)
	}
	if start == 0 {
		message = append(message, []tellraw.Message{
			{Text: "<", Color: tellraw.Yellow},
			{Text: "上一页", Color: tellraw.Gray},
		}...)
	} else {
		message = append(message, []tellraw.Message{
			{Text: "<", Color: tellraw.Yellow},
			{Text: "上一页", Color: tellraw.Green,
				ClickEvent: &tellraw.ClickEvent{
					Action: tellraw.RunCommand,
					GoFunc: func(s string, i int) {
						pager(s, list[max(0, start-BackupPlugin_PageSize)])
					},
				},
			},
		}...)
	}
	message = append(message, tellraw.Message{
		Text: "|", Color: tellraw.Yellow, Bold: true,
	})
	if len(list)-end <= 1 {
		message = append(message, []tellraw.Message{
			{Text: "下一页", Color: tellraw.Gray},
			{Text: ">", Color: tellraw.Yellow},
		}...)
	} else {
		message = append(message, []tellraw.Message{
			{Text: "下一页", Color: tellraw.Green,
				ClickEvent: &tellraw.ClickEvent{
					Action: tellraw.RunCommand,
					GoFunc: func(s string, i int) {
						pager(s, list[min(len(list)-1, end)])
					},
				},
			},
			{Text: ">", Color: tellraw.Yellow},
		}...)
	}
	bp.Tellraw("@a", message)
}

func (bp *BackupPlugin) rollbackSelected(player string, name string) {
	rollbackRequest := &RollbackWorldPending{player: player, name: name}
	rollbackRequest.Start(bp)
}

func (bp *BackupPlugin) rollbackPlayerdataSelected(player string, name string) {
	rollbackRequest := &RollbackPlayerdataPending{player: player, name: name}
	rollbackRequest.Start(bp)
}

func (bp *BackupPlugin) getBackupList(dirEntry []fs.DirEntry) []string {
	slices.SortFunc(dirEntry, func(a fs.DirEntry, b fs.DirEntry) int {
		stata, err := a.Info()
		if err != nil {
			return 0
		}
		statb, err := b.Info()
		if err != nil {
			return 0
		}
		return statb.ModTime().Compare(stata.ModTime())
	})
	return lo.Map(dirEntry, func(item fs.DirEntry, index int) string {
		return item.Name()
	})
}

func (bp *BackupPlugin) rollbackPlayerdataList(player string, start string) {
	pi, err := bp.GetPlayerInfo(player)
	if err != nil {
		bp.TellrawError("@a", err)
		return
	}
	backupFiles, err := os.ReadDir(filepath.Join(bp.Dest, "playerdata", pi.UUID))
	if err != nil {
		bp.TellrawError("@a", err)
		return
	}
	backupList := bp.getBackupList(backupFiles)
	if len(backupList) == 0 {
		bp.Tellraw("@a", []tellraw.Message{{Text: "无可用备份", Color: tellraw.Red}})
		return
	}
	index := slices.Index(backupList, start)
	if index < 0 {
		index = 0
	}
	if index >= len(backupList) {
		bp.Tellraw("@a", []tellraw.Message{{Text: "无可用备份", Color: tellraw.Red}})
		return
	}
	bp.showList(backupList, index, min(len(backupList), index+BackupPlugin_PageSize), func(selected string) tellraw.GoFunc {
		return func(triggerplayer string, i int) {
			if triggerplayer != player {
				bp.Tellraw(triggerplayer, []tellraw.Message{
					{Text: "该列表仅能由请求回档的玩家进行选择", Color: tellraw.Red},
				})
				return
			}
			bp.rollbackPlayerdataSelected(player, selected)
		}
	}, bp.rollbackPlayerdataList)
}

func (bp *BackupPlugin) rollbackList(_ string, start string) {
	backupFiles, err := os.ReadDir(filepath.Join(bp.Dest, "world"))
	if err != nil {
		bp.TellrawError("@a", err)
		return
	}
	backupList := bp.getBackupList(backupFiles)
	if len(backupList) == 0 {
		bp.Tellraw("@a", []tellraw.Message{{Text: "无可用备份", Color: tellraw.Red}})
		return
	}
	index := slices.Index(backupList, start)
	if index < 0 {
		index = 0
	}
	if index >= len(backupList) {
		bp.Tellraw("@a", []tellraw.Message{{Text: "无可用备份", Color: tellraw.Red}})
		return
	}
	bp.showList(backupList, index, min(len(backupList), index+BackupPlugin_PageSize), func(selected string) tellraw.GoFunc {
		return func(player string, i int) {
			bp.rollbackSelected(player, selected)
		}
	}, bp.rollbackList)
}

func (bp *BackupPlugin) Confirm(player string) {
	bp.rollbackLock.RLock()
	rb := bp.rollbackPending
	bp.rollbackLock.RUnlock()
	if rb != nil {
		rb.Comfirm(player)
	} else {
		bp.Tellraw("@a", []tellraw.Message{
			{Text: "没有正在进行的回档请求", Color: tellraw.Red},
		})
	}
}

func (bp *BackupPlugin) Cancel(player string) {
	bp.rollbackLock.RLock()
	rb := bp.rollbackPending
	bp.rollbackLock.RUnlock()
	if rb != nil {
		rb.Abort(player)
	} else {
		bp.Tellraw("@a", []tellraw.Message{
			{Text: "没有正在进行的回档请求", Color: tellraw.Red},
		})
	}
}

func (bp *BackupPlugin) Cli(player string, args ...string) {
	if len(args) == 0 {
		args = []string{"help"}
	}
	switch args[0] {
	case "make":
		if len(args) < 2 {
			bp.Tellraw("@a", []tellraw.Message{{Text: "没有填写备注", Color: tellraw.Red}})
			return
		}
		bp.MakeBackup(strings.Join(args[1:], " "))
	case "rollback":
		if len(args) < 2 {
			bp.rollbackList(player, "")
			return
		} else {
			bp.rollbackSelected(player, strings.Join(args[1:], " "))
		}
	case "rollbackplayerdata":
		if len(args) < 2 {
			bp.rollbackPlayerdataList(player, "")
			return
		} else {
			bp.rollbackPlayerdataSelected(player, strings.Join(args[1:], " "))
		}
	case "cancel":
		bp.Cancel(player)
	case "confirm":
		bp.Confirm(player)
	default:
		bp.Tellraw("@a", []tellraw.Message{
			{Text: "可用命令如下:\n", Color: tellraw.Light_Purple},
			{Text: "!!backup ", Color: tellraw.Red},
			{Text: "make ", Color: tellraw.Yellow},
			{Text: "<备注> ", Color: tellraw.Green},
			{Text: "创建名为备注的备份\n", Color: tellraw.Light_Purple},
			{Text: "!!backup ", Color: tellraw.Red},
			{Text: "rollback ", Color: tellraw.Yellow},
			{Text: "回档整个世界\n", Color: tellraw.Light_Purple},
			{Text: "!!backup ", Color: tellraw.Red},
			{Text: "rollbackplayerdata ", Color: tellraw.Yellow},
			{Text: "回档当前玩家的数据", Color: tellraw.Light_Purple},
		})
	}

}

func (bp *BackupPlugin) Init(pm pluginabi.PluginManager) (err error) {
	bp.pm = pm

	bp.cron, _ = gocron.NewScheduler()
	bp.cron.NewJob(gocron.CronJob("*/30 * * * *", false), gocron.NewTask(func() {
		if len(bp.GetPlayerList()) > 0 {
			bp.MakeBackup("AutoBackup")
		}
	}), gocron.WithSingletonMode(gocron.LimitModeReschedule))
	err = bp.BasePlugin.Init(pm, bp)
	if err != nil {
		return err
	}
	bp.RegisterCommand("backup", bp.Cli)
	return nil
}

func (bp *BackupPlugin) Start() {
	bp.cron.Start()
	if bp.backupPlayerdataTicker == nil {
		bp.backupPlayerdataTicker = time.NewTicker(60 * time.Second)
		go func() {
			for range bp.backupPlayerdataTicker.C {
				if len(bp.GetPlayerList()) > 0 {
					bp.MakePlayerDataBackup()
				}
			}
		}()
	} else {
		bp.backupPlayerdataTicker.Reset(60 * time.Second)
	}
	bp.MakePlayerDataBackup()
}

func (bp *BackupPlugin) Pause() {
	bp.cron.StopJobs()
	bp.backupPlayerdataTicker.Stop()
}
