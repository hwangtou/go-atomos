package main

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	accessLog := func(s string) { os.Stdin.WriteString(s) }
	errLog := func(s string) { os.Stdin.WriteString(s) }
	atomos.InitCosmosProcess(accessLog, errLog)
	if err := atomos.SharedCosmosProcess().Start(&AtomosRunnable); err != nil {
		atomos.SharedLogging().PushLogging(nil, atomos.LogLevel_Err, fmt.Sprintf("Start failed. err=(%v)", err))
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	if err := atomos.SharedCosmosProcess().Stop(); err != nil {
		atomos.SharedLogging().PushLogging(nil, atomos.LogLevel_Err, fmt.Sprintf("Stop failed. err=(%v)", err))
	}
	<-time.After(1 * time.Second)
}
