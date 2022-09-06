package main

import (
	"log"
	"os"
	"time"
)

func main() {
	ch := make(chan os.Signal)
	for {
		select {
		case sg := <-ch:
			log.Println("Sg Exit, ", sg)
			return
		case <-time.After(100 * time.Second):
			log.Println("Timer Exit")
		}
	}
}
