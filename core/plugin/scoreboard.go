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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"github.com/cespare/xxhash/v2"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
)

type ScoreboardCore struct {
	BasePlugin
	score     map[string]map[string]int64
	scorelist []string
	lock      sync.RWMutex
	debounce  *time.Timer
}

func (sc *ScoreboardCore) Init(pm pluginabi.PluginManager) error {
	sc.BasePlugin.Init(pm, sc)
	sc.score = make(map[string]map[string]int64)
	return nil
}

func (sc *ScoreboardCore) ensureScoreboard(context pluginabi.PluginName, name string, criterion string, displayname string) {
	name = fmt.Sprintf("%s_%s", sc.getNamespace(context), name)
	sc.lock.RLock()
	ok := slices.Contains(sc.scorelist, name)
	sc.lock.RUnlock()
	if ok {
		return
	}
	sc.Println(
		color.YellowString("插件 "),
		color.BlueString(context.DisplayName()),
		color.YellowString(" 注册了一个 "),
		color.GreenString(name[9:]),
		color.YellowString("("),
		color.HiCyanString(displayname),
		color.YellowString(")"),
		color.YellowString("["),
		color.CyanString(criterion),
		color.YellowString("]"),
		color.YellowString("记分板"),
	)
	sc.RunCommand(fmt.Sprintf(`scoreboard objectives add %s %s "%s"`, name, criterion, displayname))
	sc.lock.Lock()
	sc.scorelist = append(sc.scorelist, name)
	sc.lock.Unlock()
	sc.requestSync()
}

func (sc *ScoreboardCore) getNamespace(context pluginabi.PluginName) string {
	return fmt.Sprintf("%x", xxhash.Sum64String(context.Name()))[8:]
}

func (sc *ScoreboardCore) Name() string {
	return "ScoreboardCore"
}

func (sc *ScoreboardCore) DisplayName() string {
	return "记分板核心"
}

var ScoreboardTrackedPlayer = regexp.MustCompile(`There are \d tracked .*?:\s?(.*)`)
var ScoreboardTrackedPlayerScore = regexp.MustCompile(`^.*? has (\d+)`)

func (sc *ScoreboardCore) requestSync() {
	if sc.debounce != nil {
		sc.debounce.Reset(1 * time.Second)
	}
	sc.debounce = time.AfterFunc(1*time.Second, sc.syncScore)
}

func (sc *ScoreboardCore) displayScoreboard(context pluginabi.PluginName, name string, slot string) {
	name = fmt.Sprintf("%s_%s", sc.getNamespace(context), name)
	sc.lock.RLock()
	ok := slices.Contains(sc.scorelist, name)
	sc.lock.RUnlock()
	if !ok {
		return
	}
	sc.RunCommand(fmt.Sprintf(`scoreboard objectives setdisplay %s %s`, slot, name))
}

func (sc *ScoreboardCore) scoreAction(context pluginabi.PluginName, player string, name string, action string, count int64) {
	name = fmt.Sprintf("%s_%s", sc.getNamespace(context), name)
	sc.lock.RLock()
	ok := slices.Contains(sc.scorelist, name)
	sc.lock.RUnlock()
	if !ok {
		return
	}
	sc.RunCommand(fmt.Sprintf(`scoreboard players %s %s %s %d`, action, player, name, count))
}

func (sc *ScoreboardCore) getOneScore(context pluginabi.PluginName, player string, name string) int64 {
	sc.syncOneScore(context, player, name)
	sc.lock.RLock()
	defer sc.lock.RUnlock()
	if playerscope, ok := sc.score[player]; ok {
		if score, ok := playerscope[name]; ok {
			return score
		}
	}
	return 0
}

func (sc *ScoreboardCore) getAllScore() (scores map[string]map[string]int64) {
	scores = map[string]map[string]int64{}
	sc.syncScore()
	sc.lock.RLock()
	maps.Copy(scores, sc.score)
	sc.lock.RUnlock()
	return scores
}

func (sc *ScoreboardCore) syncOneScore(context pluginabi.PluginName, player string, name string) {
	name = fmt.Sprintf("%s_%s", sc.getNamespace(context), name)
	sc.lock.RLock()
	ok := slices.Contains(sc.scorelist, name)
	sc.lock.RUnlock()
	if !ok {
		return
	}

	scoreResult := sc.RunCommand(fmt.Sprintf(`scoreboard players get %s %s`, player, name))
	sc.lock.Lock()
	defer sc.lock.Unlock()
	scoreMatch := ScoreboardTrackedPlayerScore.FindStringSubmatch(scoreResult)
	if len(scoreMatch) == 2 {
		scoreValue, err := strconv.ParseInt(scoreMatch[1], 10, 64)
		if err == nil {
			sc.score[player][name] = scoreValue
		}
	}
}

func (sc *ScoreboardCore) clearTrigger() {
	triggerListStr := sc.RunCommand("scoreboard objectives list")
	triggerStrList := strings.Split(triggerListStr, ":")
	if len(triggerStrList) == 2 {
		triggerStrList := lo.Map(strings.Split(triggerStrList[1], ","), func(item string, index int) string {
			return strings.Trim(strings.TrimSpace(item), "[]")
		})
		for _, trigger := range triggerStrList {
			if strings.HasPrefix(trigger, "manager_trigger") {
				sc.RunCommand("scoreboard objectives remove " + trigger)
			}
		}
	}
}

func (sc *ScoreboardCore) syncScore() {
	trackedPlayersStr := sc.RunCommand("scoreboard players list")
	if ScoreboardTrackedPlayer.MatchString(trackedPlayersStr) {
		trackedPlayers := lo.Map(strings.Split(ScoreboardTrackedPlayer.FindStringSubmatch(trackedPlayersStr)[1], ","), func(item string, index int) string {
			return strings.TrimSpace(item)
		})
		sc.lock.Lock()
		defer sc.lock.Unlock()
		for _, player := range trackedPlayers {
			if _, ok := sc.score[player]; !ok {
				sc.score[player] = make(map[string]int64)
			}
			for _, score := range sc.scorelist {
				scoreResult := sc.RunCommand(fmt.Sprintf(`scoreboard players get %s %s`, player, score))
				scoreMatch := ScoreboardTrackedPlayerScore.FindStringSubmatch(scoreResult)
				if len(scoreMatch) == 2 {
					scoreValue, err := strconv.ParseInt(scoreMatch[1], 10, 64)
					if err == nil {
						sc.score[player][score] = scoreValue
					}
				}
			}
		}
	}
}

func (sc *ScoreboardCore) Start() {
	sc.clearTrigger()
}
