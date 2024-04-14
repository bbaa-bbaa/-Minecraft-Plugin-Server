package main

import (
	"cgit.bbaa.fun/bbaa/minecraft-plugin-server/core"
)

func main() {
	minecraftManagerClient := &core.MinecraftPluginManager{StartScript: "/home/bbaa/Minecraft/TestNeoforgeServer/run.sh"}
	minecraftManagerClient.Dial("127.0.0.1:12345")
	select {}
}
