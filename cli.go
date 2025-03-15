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

package main

import (
	"flag"
	"time"

	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/core"
	"git.bbaa.fun/bbaa/minecraft-plugin-daemon/plugins"
)

var StartScript = flag.String("script", "/home/bbaa/Minecraft/FabricBase/run.sh", "start")

func main() {
	flag.Parse()
	go func() {
		for {
			err := createGameManager()
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			break
		}
	}()
	select {}
}

func createGameManager() error {
	minecraftManagerClient := &core.MinecraftPluginManager{StartScript: *StartScript}
	err := minecraftManagerClient.Dial("127.0.0.1:12345")
	if err != nil {
		return err
	}
	minecraftManagerClient.RegisterPlugin(&plugins.TeleportPlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.HomePlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.BackPlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.BackupPlugin{Source: "/home/bbaa/Minecraft/FabricBase/world", Dest: "/home/bbaa/Minecraft/Backup/"})
	minecraftManagerClient.RegisterPlugin(&plugins.StatusPlugin{MaxSentBandwidth: 50, MaxRecvBandwidth: 800})
	return nil
}
