package main

import (
	"github.com/golang/protobuf/proto"
	"github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/demo/helloworld/api"
	"log"
	"time"
)

func boostrap() error {
	return nil
}

type hello struct {
	self atomos.AtomSelf
}

func (h *hello) Spawn(self atomos.AtomSelf, data []byte) error {
	h.self = self

	return nil
}

func (h *hello) Close(task map[uint64]proto.Message) []byte {
	h.self.Log().Info("Close") // todo log failed
	return nil
}

func (h *hello) NowTask(t *api.HelloRequest) {
	h.self.Log().Info("Now Task") // todo log failed
	h.self.Task().After(time.Second, h.DelayTask, &api.HelloRequest{})
}

func (h *hello) DelayTask(t *api.HelloRequest) {
	h.self.Log().Info("Delay Task")
	h.self.Halt()
}

func (h *hello) SayHello(from atomos.Id, in *api.HelloRequest) (*api.HelloReply, error) {
	h.self.Task().Add(h.NowTask, &api.HelloRequest{})
	return &api.HelloReply{ Message: "Reply" }, nil
}

func main() {
	cos, err := atomos.NewCosmos(atomos.CosmosConfig{
		CosmosId:        "server",
		NodeId:            "1",
		DataFilePath:    "/tmp/cosmos/server_1",
		Cluster: &atomos.CosmosClusterConfig{
			ListenNetwork:   "tcp",
			ListenAddress:   "0.0.0.0:10021",
			ConnHeartbeat:   10,
			ConnHeaderSize:    8,
			EtcdEndpoints:   []string{"http://127.0.0.1:2379"},
			EtcdUsername:    "",
			EtcdPassword:    "",
			EtcdDialTimeout: 5,
			EtcdTtl:         5,
		},
	}, &api.GreeterDesc)
	if err != nil {
		log.Fatalln("new cosmos", err)
	}
	if _, err := api.SpawnGreeter(cos, "a", &hello{}); err != nil {
		log.Fatalln(err)
	}
	go func() {
		time.Sleep(1 * time.Second)
		call, err := api.GetGreeterId(cos, "a")
		if err != nil {
			log.Fatalln(err)
		}
		reply, err := call.SayHello(nil, &api.HelloRequest{ Name: "Hello World!" })
		if err != nil {
			log.Fatalln(err)
		}
		log.Println(reply)
	}()
	cos.Run() // blocking
	defer cos.Close()
}
