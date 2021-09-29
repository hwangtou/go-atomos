package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

func main() {
	u := url.URL{
		Host: "localhost:20000",
		Path: "/worm",
		Scheme: "ws",
	}
	dialer := websocket.Dialer{}
	header := http.Header{}
	header.Add("worm", "a")
	conn, _, err := dialer.Dial(u.String(), header)
	if err != nil {
		panic(err)
	}
	run, mutex := true, sync.Mutex{}
	curId, session := int32(0), map[int32]*api.WormPackage{}
	go func() {
		defer func() {
			mutex.Lock()
			if run {
				conn.Close()
				conn = nil
				run = false
			}
			mutex.Unlock()
		}()
		for {
			_, buf, err := conn.ReadMessage()
			if err != nil {
				log.Printf("\nread error, err=%v", err)
				return
			}
			pack := &api.WormPackage{}
			err = json.Unmarshal(buf, pack)
			if err != nil {
				log.Printf("\nunmarshal error, err=%v", err)
				return
			}
			mutex.Lock()
			if p, has := session[pack.Id]; has {
				log.Printf("\nresponse, id=%d,req=%s,resp=%s", pack.Id, string(p.Buf), string(pack.Buf))
				delete(session, pack.Id)
			} else {
				log.Printf("\nsession not found, pack=%+v,buf=%s", pack, string(buf))
			}
			mutex.Unlock()
		}
	}()
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter Cmd: ")
		txt, err := reader.ReadString('\n')
		if err != nil {
			mutex.Lock()
			if run {
				conn.Close()
				conn = nil
				run = false
			}
			mutex.Unlock()
			return
		}
		str := strings.Split(txt, ";")
		if len(str) != 2 {
			mutex.Lock()
			if run {
				conn.Close()
				conn = nil
				run = false
			}
			mutex.Unlock()
			return
		}
		handler, value := str[0], str[1]
		fmt.Println(len(value))
		if len(value) > 1 {
			value = value[:len(value)-1]
		}
		curId += 1
		pack := &api.WormPackage{
			Id:      curId,
			Handler: handler,
			Buf:     []byte(value),
		}
		buf, err := json.Marshal(pack)
		if err != nil {
			mutex.Lock()
			if run {
				conn.Close()
				conn = nil
				run = false
			}
			mutex.Unlock()
			return
		}
		err = conn.WriteMessage(websocket.BinaryMessage, buf)
		if err != nil {
			mutex.Lock()
			if run {
				conn.Close()
				conn = nil
				run = false
			}
			mutex.Unlock()
			return
		}
	}
}
