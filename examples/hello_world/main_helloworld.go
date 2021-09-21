package main

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
	"net"
	"net/http"
	"sync"
)

func main() {
	runnable := atomos.CosmosRunnable{}
	// TaskBooth
	runnable.AddElementImplementation(api.GetTaskBoothImplement(&element.TaskBoothElement{}))
	// RemoteBooth
	runnable.AddElementInterface(api.GetRemoteBoothInterface(&element.RemoteBoothElement{}))
	// LocalBooth
	runnable.AddElementImplementation(api.GetLocalBoothImplement(&element.LocalBoothElement{}))
	// WormBooth
	runnable.AddElementImplementation(api.GetWormBoothImplement(&element.WormholeBoothElement{}))
	runnable.AddWormhole(&wormhole{})
	runnable.SetScript(scriptHelloWorld)
	config := &atomos.Config{
		Node:     api.NodeHelloWorld,
		LogPath:  "/tmp/cosmos_log/",
		LogLevel: atomos.LogLevel_Debug,
		EnableCert: &atomos.CertConfig{
			CertPath:           api.CertPath,
			KeyPath:            api.KeyPath,
			InsecureSkipVerify: true,
		},
		EnableServer: &atomos.RemoteServerConfig{
			Host: api.NodeHost,
			Port: api.NodeHelloPort,
		},
	}
	// Cycle
	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()
	exitCh, err := cosmos.Daemon(config)
	if err != nil {
		return
	}
	cosmos.SendRunnable(runnable)
	// Exit
	<-exitCh
}

func scriptHelloWorld(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	//demoTaskBooth(cosmos, mainId, killNoticeChannel)
	//demoRemoteBoothLocal(cosmos, mainId, killNoticeChannel)
	demoWormBooth(cosmos, mainId, killNoticeChannel)
}

// TaskBooth
func demoTaskBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	// Try to spawn a TaskBooth atom.
	taskBoothId, err := api.SpawnTaskBooth(cosmos.Local(), "Demo", nil)
	if err != nil {
		panic(err)
	}
	// Kill when exit.
	defer func() {
		mainId.Log().Info("Exiting")
		if err := taskBoothId.Kill(mainId); err != nil {
			mainId.Log().Info("Exiting error, err=%v", err)
		}
	}()

	// Start task demo.
	if _, err = taskBoothId.StartTask(mainId, &api.StartTaskReq{}); err != nil {
		panic(err)
	}
	<-killNoticeChannel
}

// RemoteBooth
func demoRemoteBoothLocal(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	// Try to connect to remote node.
	_, err := cosmos.Connect(api.RemoteName, api.NodeRemoteAddr)
	if err != nil {
		panic(err)
	}
	// Spawn LocalBooth atom.
	_, err = api.SpawnLocalBooth(cosmos.Local(), api.LocalBoothMainAtomName, nil)
	if err != nil {
		panic(err)
	}
	<-killNoticeChannel
}

func demoWormBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	<-killNoticeChannel
}

// Wormhole

type wormhole struct {
	mainId   atomos.MainId
	handler  *http.ServeMux
	server   *http.Server
	listener net.Listener
	upgrade  websocket.Upgrader
	mutex sync.Mutex
	worms map[string]*worm
}

func (w *wormhole) Name() string {
	return "ws"
}

func (w *wormhole) Boot() (err error) {
	addr := "0.0.0.0:20000"
	w.handler = http.NewServeMux()
	w.handler.HandleFunc("/worm", w.handleWorm)
	w.server = &http.Server{
		Addr: addr,
		Handler: w.handler,
	}
	if w.listener, err = net.Listen("tcp", addr); err != nil {
		w.server = nil
		w.listener = nil
		return err
	}
	w.worms = map[string]*worm{}
	return nil
}

func (w *wormhole) BootFailed() {
	if w.listener != nil {
		w.listener.Close()
		w.listener = nil
	}
}

func (w *wormhole) Start(id atomos.MainId) {
	w.mainId = id
	w.server.Serve(w.listener)
}

func (w *wormhole) Stop() {
	if w.server != nil {
		w.server.Close()
		w.server = nil
	}
	if w.listener != nil {
		w.listener.Close()
		w.listener = nil
	}
}

func (w *wormhole) GetWorm(name string) atomos.Worm {
	return &worm{}
}

func (w *wormhole) handleWorm(writer http.ResponseWriter, request *http.Request) {
	name := request.Header.Get("worm")
	if name == "" {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write([]byte("invalid name"))
		return
	}
	conn, err := w.upgrade.Upgrade(writer, request, nil)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write([]byte("upgrade failed"))
		return
	}
	w.mutex.Lock()
	ww, has := w.worms[name]
	if !has {
		ww = &worm{ name: name }
		w.worms[name] = ww
	}
	id, err := api.GetWormBoothId(w.mainId.Cosmos(), name)
	if err != nil {
		id, err = api.SpawnWormBooth(w.mainId.Cosmos(), name, &api.WormBoothSpawnArg{})
		if err != nil {
			w.mutex.Unlock()
			w.mainId.Log().Error("Wormhole error, err=%v", err)
			return
		}
	}
	w.mutex.Unlock()
	if has {
		ww.KickOld()
	}
	ww.mainId = w.mainId
	ww.id = id
	ww.conn = conn
	ww.AcceptNew()
	go func() {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		defer func() {
			w.mutex.Lock()
			delete(w.worms, ww.Name())
			w.mutex.Unlock()
			ww.KickOld()
			ww.Mutex.Lock()
			if ww.conn != nil {
				ww.conn.Close()
				ww.conn = nil
			}
			ww.Mutex.Unlock()
		}()
		for {
			_, msg, err := ww.conn.ReadMessage()
			if err != nil {
				return
			}
			if err = ww.Receive(msg); err != nil {
				return
			}
		}
	}()
}

type worm struct {
	sync.Mutex
	name   string
	mainId atomos.MainId
	id     api.WormBoothId
	conn   *websocket.Conn
}

func (w *worm) Receive(bytes []byte) error {
	req := &api.WormPackage{}
	if err := json.Unmarshal(bytes, req); err != nil {
		return err
	}
	resp, err := w.id.NewPackage(w.mainId, req)
	if err != nil {
		return err
	}
	respBuf, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.Send(respBuf)
}

func (w *worm) Send(bytes []byte) error {
	w.Mutex.Lock()
	defer w.Mutex.Unlock()
	if w.conn == nil {
		return errors.New("conn closed")
	}
	return w.conn.WriteMessage(websocket.TextMessage, bytes)
}

func (w *worm) Name() string {
	return w.name
}

func (w *worm) KickOld() {
}

func (w *worm) AcceptNew() {
}
