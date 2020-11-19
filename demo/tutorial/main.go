package main

import (
	"github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/demo/helloworld"
	"log"
	"time"
)

func boostrap() error {
	return nil
}

type hello struct {
	self atomos.AtomController
}

func (h *hello) Spawn(self atomos.AtomController, data []byte) {
	h.self = self
}

func (h *hello) Save() []byte {
	return []byte{}
}

func (h *hello) SayHello(in *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	return &helloworld.HelloReply{ Message: "Reply" }, nil
}

func main() {
	cos, err := atomos.NewCosmos(atomos.CosmosConfig{
		CosmosId:        "server",
		Node:            "1",
		DataFilepath:    "/tmp/cosmos/server_1",
		ListenNetwork:   "tcp",
		ListenAddress:   "0.0.0.0:10021",
		ConnHeartBeat:   10,
		ConnHeadSize:    8,
		EtcdEndpoints:   []string{"http://127.0.0.1:2379"},
		EtcdUsername:    "",
		EtcdPassword:    "",
		EtcdDialTimeout: 5,
		EtcdTTL:         5,
	}, &helloworld.GreeterDesc)
	if err != nil {
		log.Fatalln("new cosmos", err)
	}
	if _, err := helloworld.SpawnGreeterAtom(cos, "a", &hello{}); err != nil {
		log.Fatalln(err)
	}
	go func() {
		time.Sleep(1 * time.Second)
		call, err := helloworld.GetGreeterCallable(cos, "a")
		if err != nil {
			log.Fatalln(err)
		}
		reply, err := call.SayHello(&helloworld.HelloRequest{ Name: "Hello World!" })
		if err != nil {
			log.Fatalln(err)
		}
		log.Println(reply)
	}()
	cos.Run() // blocking
	defer cos.Close()
}
