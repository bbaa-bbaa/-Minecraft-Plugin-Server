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

	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/core"
	"cgit.bbaa.fun/bbaa/minecraft-plugin-daemon/plugins"
)

var StartScript = flag.String("script", "", "start")

func main() {
	flag.Parse()
	minecraftManagerClient := &core.MinecraftPluginManager{StartScript: *StartScript}
	minecraftManagerClient.Dial("127.0.0.1:12345")
	minecraftManagerClient.RegisterPlugin(&plugins.TeleportPlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.HomePlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.BackPlugin{})
	minecraftManagerClient.RegisterPlugin(&plugins.StatusPlugin{})
	select {}
}
