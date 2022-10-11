package main

import (
	go_atomos "github.com/hwangtou/go-atomos"
	"log"
	"time"
)

func main() {
	daemon, err := go_atomos.NewNodeDaemon("hello_cosmos", "cosmos1")
	if err != nil {
		log.Fatalln(err)
	}
	if daemon.BootOrDaemon() {
		if _, err = daemon.BootProcess(); err != nil {
			log.Panicln(err.AutoStack(nil, nil))
		}

		return
	}
	pid, err := daemon.DaemonProcess()
	if err != nil {
		log.Panicln(err.AutoStack(nil, nil))
		return
	}
	defer daemon.DaemonProcessClose()

	_ = pid

	idx := 0
	for {
		time.Sleep(10 * time.Second)
		log.Println("OK")
		idx += 1
		if idx == 10 {
			panic("quit")
		}
	}
}
