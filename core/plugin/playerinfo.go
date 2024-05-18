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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core/plugin/pluginabi"
	"github.com/fatih/color"
	"github.com/samber/lo"
)

type MinecraftPlayerInfo_Extra map[string]any

func (extra *MinecraftPlayerInfo_Extra) UnmarshalJSON(data []byte) error {
	var extraDecodeRaw map[string]json.RawMessage
	loadedExtra := make(MinecraftPlayerInfo_Extra)
	err := json.Unmarshal(data, &extraDecodeRaw)
	if err != nil {
		return err
	}
	for k, v := range extraDecodeRaw {
		loadedExtra[k] = v
	}
	*extra = loadedExtra
	return nil

}

type MinecraftPlayerInfo struct {
	Player       string
	Location     *MinecraftPosition
	LastLocation *MinecraftPosition
	UUID         string
	Extra        MinecraftPlayerInfo_Extra
	lock         sync.RWMutex
	playerInfo   *PlayerInfo
}

func (mpi *MinecraftPlayerInfo) Commit() error {
	mpi.lock.RLock()
	defer mpi.lock.RUnlock()
	return mpi.playerInfo.Commit(mpi)
}

func (mpi *MinecraftPlayerInfo) GetExtra(context pluginabi.PluginName, v any) error {
	mpi.lock.RLock()
	extra, ok := mpi.Extra[context.Name()]
	mpi.lock.RUnlock()
	if ok {
		switch extra := extra.(type) {
		case json.RawMessage:
			err := json.Unmarshal(extra, v)
			if err != nil {
				return err
			}
			mpi.lock.Lock()
			mpi.Extra[context.Name()] = v
			mpi.lock.Unlock()
		default:
			x := reflect.ValueOf(extra)
			if x.Kind() == reflect.Ptr {
				reflect.ValueOf(v).Elem().Set(x.Elem())
			} else {
				reflect.ValueOf(v).Elem().Set(x)
			}
		}
	}
	return nil
}

func (mpi *MinecraftPlayerInfo) PutExtra(context pluginabi.PluginName, extra any) {
	mpi.lock.Lock()
	defer mpi.lock.Unlock()
	mpi.Extra[context.Name()] = extra
}

type PlayerInfo_Storage struct {
	PlayerInfo     map[string]*MinecraftPlayerInfo
	playerInfoLock sync.RWMutex
	UUIDMap        map[string]string
	uuidMapLock    sync.RWMutex
}

func (s *PlayerInfo_Storage) Lock() {
	s.playerInfoLock.Lock()
	s.uuidMapLock.Lock()
}

func (s *PlayerInfo_Storage) Unlock() {
	s.playerInfoLock.Unlock()
	s.uuidMapLock.Unlock()
}

func (s *PlayerInfo_Storage) RLock() {
	s.playerInfoLock.RLock()
	s.uuidMapLock.RLock()
}

func (s *PlayerInfo_Storage) RUnlock() {
	s.playerInfoLock.RUnlock()
	s.uuidMapLock.RUnlock()
}

type PlayerInfo struct {
	BasePlugin
	playerList     []string
	playerListLock sync.RWMutex
	data           *PlayerInfo_Storage
}

var PlayerEnterLeaveMessage = regexp.MustCompile(`(left|joined) the game`)

func (pi *PlayerInfo) Init(pm pluginabi.PluginManager) (err error) {
	err = pi.BasePlugin.Init(pm, pi)
	if err != nil {
		return err
	}
	pi.data = &PlayerInfo_Storage{PlayerInfo: map[string]*MinecraftPlayerInfo{}, UUIDMap: map[string]string{}}
	pm.RegisterLogProcesser(pi, pi.playerJoinLeaveEvent)
	err = pi.Load()
	if err != nil {
		pi.Println(color.RedString("加载存储的玩家数据失败"))
	}
	return nil
}

func (pi *PlayerInfo) playerJoinLeaveEvent(log string, _ bool) {
	if PlayerEnterLeaveMessage.MatchString(log) {
		pi.updatePlayerList()
	}
}

func (pi *PlayerInfo) convertUUID(rawData []int32) (uuid string, err error) {
	if len(rawData) != 4 {
		return "", fmt.Errorf("parse UUID 失败")
	}
	rawUUID := &bytes.Buffer{}
	binary.Write(rawUUID, binary.BigEndian, rawData)
	hexUUID := fmt.Sprintf("%x", rawUUID.Bytes())
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexUUID[0:8], hexUUID[8:12], hexUUID[12:16], hexUUID[16:20], hexUUID[20:]), nil
}

func (pi *PlayerInfo) getPlayerName(uuid string) (player string, err error) {
	pi.data.uuidMapLock.RLock()
	player, ok := pi.data.UUIDMap[uuid]
	pi.data.uuidMapLock.RUnlock()
	if ok {
		return player, nil
	}
	playerEntitydata := pi.RunCommand(fmt.Sprintf("data get entity %s", uuid))
	if !strings.Contains(playerEntitydata, "entity data") {
		return "", fmt.Errorf("not found")
	}
	player, _, ok = strings.Cut(playerEntitydata, " ")
	if !ok || player == uuid {
		return "", fmt.Errorf("not found")
	}
	pi.data.uuidMapLock.Lock()
	pi.data.UUIDMap[uuid] = player
	pi.data.uuidMapLock.Unlock()
	return player, nil
}

func (pi *PlayerInfo) getPlayerUUID(player string) (uuid string, err error) {
	uuidEntityData := pi.RunCommand("data get entity " + player + " UUID")
	uuidData := strings.SplitN(uuidEntityData, ":", 2)
	if len(uuidData) != 2 {
		return "", fmt.Errorf("获取 NBT 失败")
	}
	uuidNbt := strings.TrimSpace(uuidData[1])
	var tempInt int64
	uuidIntArray := lo.Map(strings.Split(strings.TrimSpace(strings.TrimRight(uuidNbt[strings.Index(uuidNbt, ";")+1:], "]")), ","), func(item string, index int) int32 {
		if err != nil {
			return 0
		}
		tempInt, err = strconv.ParseInt(strings.TrimSpace(item), 10, 32)
		return int32(tempInt)
	})
	if err != nil {
		return "", err
	}
	uuid, err = pi.convertUUID(uuidIntArray)
	if err != nil {
		return "", err
	}
	pi.data.uuidMapLock.RLock()
	_, ok := pi.data.UUIDMap[uuid]
	pi.data.uuidMapLock.RUnlock()
	if !ok {
		pi.data.uuidMapLock.Lock()
		pi.data.UUIDMap[uuid] = player
		pi.data.uuidMapLock.Unlock()
	}
	return uuid, err
}

func (pi *PlayerInfo) getPlayerPosition(player string) (position *MinecraftPosition, err error) {
	var tempFloat float64
	entityPosRes := pi.RunCommand("data get entity " + player + " Pos")
	entityPos := strings.SplitN(entityPosRes, ":", 2)
	if len(entityPos) != 2 {
		return nil, fmt.Errorf("获取 NBT 失败")
	}
	entityPosList := lo.Map(strings.Split(strings.Trim(entityPos[1], "[ ]"), ","), func(item string, index int) float64 {
		if err != nil {
			return 0
		}
		tempFloat, err = strconv.ParseFloat(strings.Trim(item, " d"), 64)
		return tempFloat
	})
	if err != nil {
		return nil, err
	}
	position = &MinecraftPosition{Position: [3]float64(entityPosList)}
	entityDimRes := pi.RunCommand("data get entity " + player + " Dimension")
	entityDim := strings.SplitN(entityDimRes, ":", 2)
	if len(entityDim) != 2 {
		return nil, fmt.Errorf("获取 NBT 失败")
	}
	position.Dimension = strings.Trim(entityDim[1], `" `)
	return position, err
}

func (pi *PlayerInfo) GetPlayerInfo_Position(player string) (playerInfo *MinecraftPlayerInfo, err error) {
	playerInfo, err = pi.GetPlayerInfo(player)
	if err != nil {
		return nil, err
	}
	playerInfo.Location, err = pi.getPlayerPosition(player)
	if err != nil {
		return nil, err
	}
	return playerInfo, err
}

func (pi *PlayerInfo) GetPlayerInfo(player string) (playerInfo *MinecraftPlayerInfo, err error) {
	uuid := ""
	if len(player) == 36 {
		uuid = player
		player, err = pi.getPlayerName(uuid)
		if err != nil {
			return
		}
	}
	var ok bool
	pi.data.playerInfoLock.RLock()
	playerInfo, ok = pi.data.PlayerInfo[player]
	pi.data.playerInfoLock.RUnlock()
	if !ok {
		playerInfo = &MinecraftPlayerInfo{Player: player, playerInfo: pi, Extra: make(map[string]any), UUID: uuid}
		pi.data.playerInfoLock.Lock()
		pi.data.PlayerInfo[player] = playerInfo
		pi.data.playerInfoLock.Unlock()
		defer pi.Commit(playerInfo)
	} else {
		playerInfo.lock.Lock()
		defer playerInfo.lock.Unlock()
		if playerInfo.Extra == nil {
			playerInfo.Extra = make(map[string]any)
		}
		playerInfo.playerInfo = pi
	}
	if playerInfo.UUID == "" {
		if uuid == "" {
			playerInfo.UUID, err = pi.getPlayerUUID(player)
			if err != nil {
				return nil, err
			}
		}
	}
	if playerInfo.Location == nil {
		playerInfo.Location, err = pi.getPlayerPosition(player)
		if err != nil {
			return nil, err
		}
	}
	return playerInfo, nil
}

func (pi *PlayerInfo) GetPlayerList() []string {
	pi.playerListLock.RLock()
	defer pi.playerListLock.RUnlock()
	return slices.Clone(pi.playerList)
}

func (pi *PlayerInfo) updatePlayerList() {
	playerlistMsg := pi.RunCommand("list")
	playerlistSplitText := strings.SplitN(playerlistMsg, ":", 2)
	if len(playerlistSplitText) == 2 {
		playerList := strings.Split(strings.TrimSpace(playerlistSplitText[1]), ",")
		pi.playerListLock.Lock()
		pi.playerList = lo.FilterMap(playerList, func(players string, index int) (string, bool) {
			player := strings.TrimSpace(players)
			return player, player != ""
		})
		pi.playerListLock.Unlock()
	}
}

func (pi *PlayerInfo) Start() {
	pi.updatePlayerList()
}

func (pi *PlayerInfo) Pause() {
}

func (pi *PlayerInfo) Name() string {
	return "PlayerInfo"
}

func (pi *PlayerInfo) DisplayName() string {
	return "玩家信息"
}

func (pi *PlayerInfo) Load() error {
	data, err := os.ReadFile("data/playerinfo.json")
	if err != nil {
		return err
	}
	pi.data.Lock()
	defer pi.data.Unlock()
	err = json.Unmarshal(data, &pi.data)
	if err != nil {
		return err
	}
	return nil
}

func (pi *PlayerInfo) Commit(mpi *MinecraftPlayerInfo) error {
	if mpi == nil {
		return fmt.Errorf("无玩家信息")
	}
	pi.data.RLock()
	for _, val := range pi.data.PlayerInfo {
		val.lock.RLock()
	}
	saveData, err := json.MarshalIndent(pi.data, "", "\t")
	for _, val := range pi.data.PlayerInfo {
		val.lock.RUnlock()
	}
	pi.data.RUnlock()
	if err != nil {
		return err
	}

	return os.WriteFile("data/playerinfo.json", saveData, 0644)
}
