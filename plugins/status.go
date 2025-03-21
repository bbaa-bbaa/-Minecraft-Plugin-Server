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
	"log"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/tellraw"
	"golang.org/x/exp/maps"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v3/cpu"
	load "github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

type Status_NetStat struct {
	time         time.Time
	stat         net.IOCountersStat
	lastAnnounce time.Time
}

type StatusPlugin struct {
	plugin.BasePlugin
	pm                pluginabi.PluginManager
	LastBroadcastMspt float64
	LastMspt          []float64
	ForgeTpsCommand   string
	monitorStop       chan struct{}
	MaxSentBandwidth  float64 // Mbps
	MaxRecvBandwidth  float64 // Mbps
	lastnetStat       *Status_NetStat
}

type StatusPlugin_MinecraftLoad struct {
	World string
	MSPT  float64
	TPS   float64
	index int
}

func (s *StatusPlugin) DisplayName() string {
	return "服务器监控"
}

func (s *StatusPlugin) Name() string {
	return "StatusPlugin"
}

func (s *StatusPlugin) Ping(logmsg string, iscmdrsp bool) {
	if iscmdrsp {
		return
	}
	ping := core.PlayerMessage.FindStringSubmatch(logmsg)
	if len(ping) < 3 {
		return
	}
	number, err := strconv.ParseInt(strings.TrimSpace(ping[2]), 10, 64)
	if err != nil {
		return
	}
	number += 1
	s.Tellraw("@a", []tellraw.Message{
		{Text: fmt.Sprintf("Pong! %d", number), Color: tellraw.Aqua},
	})
}

func (s *StatusPlugin) Init(pm pluginabi.PluginManager) (err error) {
	err = s.BasePlugin.Init(pm, s)
	if err != nil {
		return err
	}
	s.RegisterCommand("status", s.status)
	s.RegisterLogProcesser(s.Ping)
	s.monitorSystem()
	return nil
}

var StatusPlugin_ParseLoad = regexp.MustCompile(`(?:Dim )?(.*?)[ ]?(?:\(.*?\))?: Mean tick time:.(.*?).ms.*?TPS:.(.{6})`)

func (s *StatusPlugin) getMinecraftLoad() map[string]StatusPlugin_MinecraftLoad {
	loadList := make(map[string]StatusPlugin_MinecraftLoad)
	worldStatusPlugin := StatusPlugin_ParseLoad.FindAllStringSubmatch(s.RunCommand(s.ForgeTpsCommand), -1)
	for idx, match := range worldStatusPlugin {
		World, MSPTStr := match[1], match[2]
		World = strings.ReplaceAll(World, "(", "")
		World = strings.ReplaceAll(World, ")", "")
		MSPT, _ := strconv.ParseFloat(MSPTStr, 64)
		TPS := math.Min(20, 1000/MSPT)
		loadList[World] = StatusPlugin_MinecraftLoad{World, MSPT, TPS, idx}
	}
	return loadList
}

func (s *StatusPlugin) leastsquares(series []float64) float64 {
	xAvg := (1 + float64(len(series))) / 2
	yAvg := 0.0
	for _, val := range series {
		yAvg += val
	}
	yAvg /= float64(len(series))

	xySum := 0.0
	xSquareSum := 0.0
	for i, val := range series {
		xySum += (float64(i+1) * val)
		xSquareSum += math.Pow(float64(i+1), 2)
	}

	return (xySum - float64(len(series))*xAvg*yAvg) / (xSquareSum - float64(len(series))*math.Pow(xAvg, 2))
}

func (s *StatusPlugin) floatLevel(f float64) tellraw.Color {
	if f < 0.4 {
		return tellraw.Green
	}
	if f < 0.7 {
		return tellraw.Yellow
	}
	return tellraw.Red
}

func (s *StatusPlugin) msptLevel(mspt float64) tellraw.Color {
	if mspt < 55 {
		return tellraw.Green
	}
	if mspt < 65 {
		return tellraw.Yellow
	}
	return tellraw.Red
}

func (s *StatusPlugin) monitorSystem() {
	cpu.Percent(0, true)
	now := time.Now()
	netio, err := s.getNetio()
	if err != nil {
		return
	}
	if s.lastnetStat != nil {
		upSpeed := float64(netio.BytesSent-s.lastnetStat.stat.BytesSent) * 8.0 / float64(now.Sub(s.lastnetStat.time).Seconds()) / 1024.0 / 1024.0
		downSpeed := float64(netio.BytesRecv-s.lastnetStat.stat.BytesRecv) * 8.0 / float64(now.Sub(s.lastnetStat.time).Seconds()) / 1024.0 / 1024.0
		if (s.MaxSentBandwidth-upSpeed) < s.MaxSentBandwidth*0.2 || (s.MaxRecvBandwidth-downSpeed) < s.MaxRecvBandwidth*0.2 {
			if now.Sub(s.lastnetStat.lastAnnounce).Seconds() > 30 && now.Sub(s.lastnetStat.time).Milliseconds() > 500 {
				s.Println(color.RedString("网络过载："), color.MagentaString("%.2f", upSpeed), color.YellowString(" Mbps↑ "), color.MagentaString("%.2f", downSpeed), color.YellowString(" Mbps↓"))
				s.lastnetStat.lastAnnounce = now
				s.Tellraw(`@a`, []tellraw.Message{
					{Text: "检测到网络带宽到达上限", Color: tellraw.Red},
				})
				s.Tellraw(`@a`, []tellraw.Message{
					{Text: "地图加载可能出现延迟", Color: tellraw.Aqua},
				})
				s.Tellraw(`@a`, []tellraw.Message{
					{Text: "网络负载: ", Color: tellraw.Aqua},
				})
				s.Tellraw(`@a`, []tellraw.Message{
					{Text: "上传: ", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%.2f", upSpeed), Color: s.floatLevel(upSpeed / s.MaxSentBandwidth)},
					{Text: " Mbps", Color: tellraw.Yellow},
					{Text: "↑", Color: tellraw.Aqua},
					{Text: fmt.Sprintf("(%.2f%%)", upSpeed/s.MaxSentBandwidth*100), Color: s.floatLevel(upSpeed / s.MaxSentBandwidth)},
				})
				s.Tellraw(`@a`, []tellraw.Message{
					{Text: "下载: ", Color: tellraw.Yellow},
					{Text: fmt.Sprintf("%.2f", downSpeed), Color: s.floatLevel(downSpeed / s.MaxRecvBandwidth)},
					{Text: " Mbps", Color: tellraw.Yellow},
					{Text: "↓", Color: tellraw.Aqua},
					{Text: fmt.Sprintf("(%.2f%%)", downSpeed/s.MaxRecvBandwidth*100), Color: s.floatLevel(downSpeed / s.MaxRecvBandwidth)},
				})
			}
		}
	} else {
		s.lastnetStat = &Status_NetStat{
			time: now,
			stat: netio,
		}
	}
	s.lastnetStat.time = now
	s.lastnetStat.stat = netio
}

func (s *StatusPlugin) getNetio() (o net.IOCountersStat, err error) {
	netio, err := net.IOCounters(true)
	if err != nil {
		return
	}
	for _, nic := range netio {
		if strings.HasPrefix(nic.Name, "eth") || strings.HasPrefix(nic.Name, "en") || strings.HasPrefix(nic.Name, "wl") {
			o.PacketsSent += nic.PacketsSent
			o.PacketsRecv += nic.PacketsRecv
			o.Errin += nic.Errin
			o.Dropin += nic.Dropin
			o.BytesRecv += nic.BytesRecv
			o.BytesSent += nic.BytesSent
			o.Errout += nic.Errout
			o.Dropout += nic.Dropout
		}
	}
	o.Name = "all"
	return
}

func (s *StatusPlugin) monitorGame() {
	load := s.getMinecraftLoad()
	overall, ok := load["Overall"]
	if !ok {
		return
	}
	if len(s.LastMspt) == 4 {
		s.LastMspt = s.LastMspt[1:]
	}
	s.LastMspt = append(s.LastMspt, overall.MSPT)
	K := s.leastsquares(s.LastMspt)
	if math.Abs(K) > 2.0 {
		if K > 0 && math.Abs(slices.Max(s.LastMspt)-s.LastBroadcastMspt) > 8 {
			s.LastBroadcastMspt = slices.Max(s.LastMspt)
			s.Tellraw(`@a`, []tellraw.Message{
				{
					Text:  `检测到服务器负载增加`,
					Color: tellraw.Red,
					Bold:  true,
				},
			})
		} else if K < 0 && math.Abs(slices.Min(s.LastMspt)-s.LastBroadcastMspt) > 8 {
			s.LastBroadcastMspt = slices.Min(s.LastMspt)
			s.Tellraw(`@a`, []tellraw.Message{
				{
					Text:  `检测到服务器负载减少`,
					Color: "green",
					Bold:  true,
				},
			})
		} else {
			return
		}
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: `世界: `, Color: tellraw.Aqua},
			{Text: "服务器", Color: tellraw.Green, Bold: true},
			{Text: ` TPS: `, Color: "aqua"},
			{Text: fmt.Sprintf("%.2f", overall.TPS), Color: s.msptLevel(overall.MSPT)},
			{Text: ` MSPT: `, Color: "aqua"},
			{Text: fmt.Sprintf("%.2fms", overall.MSPT), Color: s.msptLevel(overall.MSPT)},
			{Text: ` 负载: `, Color: "aqua"},
			{Text: fmt.Sprintf(`%.2f%%`, overall.MSPT/50*100), Color: s.msptLevel(overall.MSPT)},
		})
	}
}

func (s *StatusPlugin) status(player string, args ...string) {
	now := time.Now()
	s.Tellraw(`@a`, []tellraw.Message{{Text: "============ 系统负载 ============", Color: tellraw.Green}})
	cpu_count, _ := cpu.Counts(true)
	cpu_usage, err := cpu.Percent(0, true)
	if err != nil {
		log.Panic(err)
	}
	if err == nil {
		cpu_usage_avg := lo.Reduce(cpu_usage, func(agg float64, item float64, index int) float64 {
			return agg + item
		}, 0) / float64(len(cpu_usage)) / 100.0
		usage_bar := int(math.RoundToEven(cpu_usage_avg * 32.0))
		per_cpu_usage := &tellraw.HoverEvent{
			Action: tellraw.Show_Text,
			Contents: lo.Flatten(lo.Map(cpu_usage, func(usage float64, index int) (m []tellraw.Message) {
				if index != 0 {
					m = append(m, tellraw.Message{Text: "\n"})
				}
				usage_bar := int(math.RoundToEven(usage / 100 * 32.0))
				m = append(m, []tellraw.Message{
					{Text: fmt.Sprintf("CPU #%d: ", index), Color: tellraw.Aqua},
					{Text: "[", Color: tellraw.Yellow},
					{Text: strings.Repeat("|", max(usage_bar, 0)), Color: tellraw.Red},
					{Text: strings.Repeat("|", max(32-usage_bar, 0)), Color: tellraw.Green},
					{Text: "]", Color: tellraw.Yellow},
					{Text: fmt.Sprintf(" %.2f%%", usage), Color: s.floatLevel(cpu_usage_avg)},
				}...)
				return m
			})),
		}
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "CPU使用率: ", Color: tellraw.Aqua},
			{Text: "[", Color: tellraw.Yellow},
			{Text: strings.Repeat("|", max(usage_bar, 0)), Color: tellraw.Red, HoverEvent: per_cpu_usage},
			{Text: strings.Repeat("|", max(32-usage_bar, 0)), Color: tellraw.Green, HoverEvent: per_cpu_usage},
			{Text: "]", Color: tellraw.Yellow},
			{Text: fmt.Sprintf(" %.2f%%", cpu_usage_avg*100), Color: s.floatLevel(cpu_usage_avg)},
		})
	}
	system_load, err := load.Avg()
	if err == nil && cpu_count != 0 {
		load1, load5, load15 := system_load.Load1, system_load.Load5, system_load.Load15
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "系统负载: ", Color: tellraw.Aqua},
			{Text: "1min: ", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.2f", load1), Color: s.floatLevel(load1 / float64(cpu_count))},
			{Text: " 5min: ", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.2f", load5), Color: s.floatLevel(load5 / float64(cpu_count))},
			{Text: " 15min: ", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.2f", load15), Color: s.floatLevel(load15 / float64(cpu_count))},
		})
	}
	sys_mem, err := mem.VirtualMemory()
	minecraft_status, err_minecraft := s.pm.Status()
	if err == nil && err_minecraft == nil {
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "内存占用: ", Color: tellraw.Aqua},
			{Text: fmt.Sprintf("%.0f", float64(sys_mem.Used)/1024/1024), Color: s.floatLevel(float64(sys_mem.Used) / float64(sys_mem.Total))},
			{Text: "[", Color: tellraw.Light_Purple},
			{Text: fmt.Sprintf("%.0f", float64(minecraft_status.Usedmemory)/1024/1024), Color: s.floatLevel(float64(sys_mem.Used) / float64(sys_mem.Total))},
			{Text: "]", Color: tellraw.Light_Purple},
			{Text: " MiB/", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.0f", float64(sys_mem.Total)/1024/1024), Color: tellraw.Green},
			{Text: " MiB", Color: tellraw.Yellow},
		})
	}
	netio, err := s.getNetio()
	if err == nil && s.lastnetStat != nil {
		upSpeed := float64(netio.BytesSent-s.lastnetStat.stat.BytesSent) * 8.0 / float64(now.Sub(s.lastnetStat.time).Seconds()) / 1024.0 / 1024.0
		downSpeed := float64(netio.BytesRecv-s.lastnetStat.stat.BytesRecv) * 8.0 / float64(now.Sub(s.lastnetStat.time).Seconds()) / 1024.0 / 1024.0
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "网络负载: ", Color: tellraw.Aqua},
		})
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "上传: ", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.2f", upSpeed), Color: s.floatLevel(upSpeed / s.MaxSentBandwidth)},
			{Text: " Mbps", Color: tellraw.Yellow},
			{Text: "↑", Color: tellraw.Aqua},
			{Text: fmt.Sprintf("(%.2f%%)", upSpeed/s.MaxSentBandwidth*100), Color: s.floatLevel(upSpeed / s.MaxSentBandwidth)},
		})
		s.Tellraw(`@a`, []tellraw.Message{
			{Text: "下载: ", Color: tellraw.Yellow},
			{Text: fmt.Sprintf("%.2f", downSpeed), Color: s.floatLevel(downSpeed / s.MaxRecvBandwidth)},
			{Text: " Mbps", Color: tellraw.Yellow},
			{Text: "↓", Color: tellraw.Aqua},
			{Text: fmt.Sprintf("(%.2f%%)", downSpeed/s.MaxRecvBandwidth*100), Color: s.floatLevel(downSpeed / s.MaxRecvBandwidth)},
		})
		s.lastnetStat.time = now
		s.lastnetStat.stat = netio
	}
	s.Tellraw(`@a`, []tellraw.Message{{Text: "============ 服务负载 ============", Color: tellraw.Green}})
	minecraft_load := maps.Values(s.getMinecraftLoad())
	slices.SortFunc(minecraft_load, func(a StatusPlugin_MinecraftLoad, b StatusPlugin_MinecraftLoad) int {
		return int(a.index - b.index)
	})
	for _, load := range minecraft_load {
		if load.MSPT > 1 {
			s.Tellraw(`@a`, []tellraw.Message{
				{Text: `世界: `, Color: tellraw.Aqua},
				{Text: s.GetWorldName(load.World), Color: tellraw.Green, Bold: true},
				{Text: ` TPS: `, Color: "aqua"},
				{Text: fmt.Sprintf("%.2f", load.TPS), Color: s.msptLevel(load.MSPT)},
				{Text: ` MSPT: `, Color: "aqua"},
				{Text: fmt.Sprintf("%.2fms", load.MSPT), Color: s.msptLevel(load.MSPT)},
				{Text: ` 负载: `, Color: "aqua"},
				{Text: fmt.Sprintf(`%.2f%%`, load.MSPT/50*100), Color: s.msptLevel(load.MSPT)},
			})
		}
	}
}

func (s *StatusPlugin) testTPSCommand() {
	tpsCommands := []string{"neoforge tps", "forge tps", "fabric tps"}
	for _, testcmd := range tpsCommands {
		res := s.RunCommand(testcmd)
		if !core.UnknownCommand.MatchString(res) {
			s.ForgeTpsCommand = testcmd
			return
		}
	}
}

func (s *StatusPlugin) monitorWorker() {
	monitorTicker := time.NewTicker(10 * time.Second)
	systemTicker := time.NewTicker(1 * time.Second)
	s.monitorStop = make(chan struct{}, 1)
	for {
		select {
		case <-monitorTicker.C:
			if len(s.GetPlayerList()) > 0 {
				if s.ForgeTpsCommand != "" {
					s.monitorGame()
				}
			}
		case <-systemTicker.C:
			if len(s.GetPlayerList()) > 0 {
				s.monitorSystem()
			}
		case <-s.monitorStop:
			return
		}
	}
}

func (s *StatusPlugin) Start() {
	if s.ForgeTpsCommand == "" {
		s.testTPSCommand()
	}
	go s.monitorWorker()
}

func (s *StatusPlugin) Pause() {
	if s.monitorStop != nil {
		s.monitorStop <- struct{}{}
	}
}
