package element

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type WormholeBoothElement struct {
	mainId   atomos.MainId
	server   *http.Server
	listener net.Listener
	upgrade  websocket.Upgrader
}

func (w *WormholeBoothElement) Load(mainId atomos.MainId) (err error) {
	//// Load config.
	//whConf := &api.WormholeBoothSpawnArg{}
	//if err = mainId.CustomizeConfig("Wormhole", whConf); err != nil {
	//	return err
	//}
	// Server.
	mux := http.NewServeMux()
	mux.HandleFunc("/worm", w.handleWorm)
	server := &http.Server{
		Addr: ":20000",
		Handler: mux,
		ErrorLog: log.New(w, "", 0),
	}
	// Listen.
	listener, err := net.Listen("tcp", ":20000")
	if err != nil {
		return err
	}
	w.mainId = mainId
	w.server = server
	w.listener = listener
	w.upgrade.CheckOrigin = func(r *http.Request) bool {
		return true
	}
	return nil
}

func (w *WormholeBoothElement) Unload() {
	if w.listener != nil {
		if err := w.listener.Close(); err != nil {
			w.mainId.Log().Error("Wormhole unload closing listener error, err=%v", err)
		}
		w.listener = nil
	}
	if w.server != nil {
		if err := w.server.Close(); err != nil {
			w.mainId.Log().Error("Wormhole unload closing server error, err=%v", err)
		}
		w.server = nil
	}
}

func (w *WormholeBoothElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 10
}

func (w *WormholeBoothElement) AtomConstructor() atomos.Atom {
	return &WormholeBoothAtom{}
}

func (w *WormholeBoothElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (w *WormholeBoothElement) AtomCanKill(id atomos.Id) bool {
	return true
}

func (w *WormholeBoothElement) Daemon() {
	w.mainId.Log().Info("WormholeBoothElement Daemon")
	_ = w.server.Serve(w.listener)
}

func (w *WormholeBoothElement) handleWorm(writer http.ResponseWriter, request *http.Request) {
	// Handle incoming.
	w.mainId.Log().Info("handleWorm")
	name := request.Header.Get("worm")
	if name == "" {
		//writer.WriteHeader(http.StatusBadRequest)
		//w.mainId.Log().Error("WormholeBoothElement handleWorm auth error, name=%s", name)
		//_, _ = writer.Write([]byte("invalid name"))
		//return
		name = "a"
	}
	conn, err := w.upgrade.Upgrade(writer, request, nil)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		w.mainId.Log().Error("WormholeBoothElement handleWorm upgrade error, name=%s,err=%v", name, err)
		_, _ = writer.Write([]byte("upgrade failed"))
		return
	}
	// Find existed wormhole.
	id, err := api.GetWormholeBoothId(w.mainId.Cosmos(), name)
	if err != nil {
		id, err = api.SpawnWormholeBooth(w.mainId.Cosmos(), name, nil)
		if err != nil {
			w.mainId.Log().Error("WormholeBoothElement handleWorm spawn error, name=%s,err=%v", name, err)
			return
		}
	}

	// Send wormhole to the Wormhole Atom.
	d := &wormDaemon{
		id: id,
		conn: conn,
	}
	if err = id.Accept(d); err != nil {
		return
	}
}

func (w *WormholeBoothElement) Write(l []byte) (int, error) {
	var msg string
	if len(l) > 0 {
		msg = string(l[:len(l)-1])
	}
	w.mainId.Log().Info(msg)
	return 0, nil
}

// WormDaemon

type wormDaemon struct {
	self  atomos.AtomSelf
	id    api.WormholeBoothId
	conn  *websocket.Conn
	mutex sync.Mutex
}

type Package struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

func (w *wormDaemon) Daemon(self atomos.AtomSelf) error {
	w.self = self
	defer func() {
		w.self.Log().Error("WormholeDaemon is closing, id=%v", w.id)
		if err := w.conn.Close(); err != nil {
			w.self.Log().Error("WormholeDaemon closed error, id=%v,err=%v", w.id, err)
		}
	}()
	const readTimeout = 10 * time.Second
	for {
		if readTimeout > 0 {
			if err := w.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
				w.self.Log().Error("WormholeDaemon read timeout, id=%v", w.id)
				return err
			}
		}
		msgType, msg, err := w.conn.ReadMessage()
		if err != nil {
			w.self.Log().Error("WormholeDaemon read error, id=%v,err=%v", w.id, err)
			return err
		}
		data := &struct{}{}
		switch msgType {
		case websocket.TextMessage:
			w.self.Log().Info("WormholeDaemon text, id=%v,data=%s", w.id, data)
			err = json.Unmarshal(msg, data)
		case websocket.CloseMessage:
			w.self.Log().Info("WormholeDaemon exit, id=%v,data=%s", w.id, data)
			return w.conn.WriteMessage(websocket.CloseMessage, []byte{})
		case websocket.PingMessage:
			w.self.Log().Info("WormholeDaemon ping, id=%v,data=%s", w.id, data)
			err = w.conn.WriteMessage(websocket.PongMessage, []byte{})
		}
		if err != nil {
			w.self.Log().Error("WormholeDaemon handle error, id=%v,err=%s", w.id, err)
			return err
		}
	}
}

func (w *wormDaemon) Send(bytes []byte) error {
	return w.conn.WriteMessage(websocket.TextMessage, bytes)
}

func (w *wormDaemon) Close(isKickByNew bool) error {
	return w.conn.Close()
}

type WormholeBoothAtom struct {
	self    atomos.AtomSelf
	control atomos.WormholeControl
}

// Atom

func (w *WormholeBoothAtom) Spawn(self atomos.AtomSelf, arg *api.WormholeBoothSpawnArg, data *api.WormholeBoothData) error {
	w.self = self
	return nil
}

func (w *WormholeBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	if w.control != nil {
		if err := w.control.Close(false); err != nil {
			w.self.Log().Error("Halt close controller error, err=%v", err)
		}
		w.control = nil
	}
	w.self.Log().Info("Halt")
	return nil
}

// Wormhole

func (w *WormholeBoothAtom) AcceptWorm(control atomos.WormholeControl) error {
	if w.control != nil {
		if err := w.control.Close(true); err != nil {
			w.self.Log().Error("Close kick old error, err=%v", err)
		}
		w.control = nil
	}
	w.control = control
	w.self.Log().Info("Accept worm")
	return nil
}

func (w *WormholeBoothAtom) CloseWorm(control atomos.WormholeControl) {
	if w.control != nil {
		if err := w.control.Close(true); err != nil {
			w.self.Log().Error("Close worm error, err=%v", err)
		}
		w.control = nil
	}
	w.self.Log().Info("Close worm")
}
