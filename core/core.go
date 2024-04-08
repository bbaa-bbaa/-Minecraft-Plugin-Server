package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"

	pb "cgit.bbaa.fun/bbaa/minecraft-plugin-server/core/manager"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var Client *pb.Client

func main() {
	flag.Parse()
	var opts []grpc.DialOption

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.Dial("localhost:12345", opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewManagerClient(conn)
	ctx := context.Background()
	Client, _ := client.Login(ctx, nil)
	msg, _ := client.Message(ctx, Client)

	go func() {
		for {
			message, err := msg.Recv()
			if errors.Is(err, io.EOF) {
				fmt.Println(err)
				break
			}
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println(message.Content)
			//client.Lock(ctx, Client)
			//	client.Write(ctx, &pb.WriteRequest{Content: `tellraw @a {"text":"1234"}`, Client: Client})
			//client.Unlock(ctx, Client)
		}
	}()
	client.Start(ctx, &pb.StartRequest{Path: "/home/bbaa/Minecraft/TestNeoforgeServer/run.sh", Client: Client})
	select {}
}
