package element

import (
	"errors"
	"github.com/gorilla/websocket"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"net/http"
)

type WormholeBoothElement struct {
	mainId   atomos.MainId
	server   *http.Server
	listener net.Listener
	upgrade  websocket.Upgrader
}

func (w *WormholeBoothElement) Load(mainId atomos.MainId) (err error) {
	// Load config.
	whConf := &api.WormholeBoothSpawnArg{}
	if err = mainId.CustomizeConfig("Wormhole", whConf); err != nil {
		return err
	}
	// Server.
	mux := http.NewServeMux()
	mux.HandleFunc("/worm", w.handleWorm)
	server := &http.Server{
		Addr: whConf.Addr,
		Handler: mux,
		ErrorLog: log.New(w, "", 0),
	}
	// Listen.
	listener, err := net.Listen("tcp", whConf.Addr)
	if err != nil {
		return err
	}
	w.mainId = mainId
	w.server = server
	w.listener = listener
	return nil
}

func (w *WormholeBoothElement) Unload() {
	if w.listener != nil {
		w.listener.Close()
		w.listener = nil
	}
	if w.server != nil {
		w.server.Close()
		w.server = nil
	}
}

func (w *WormholeBoothElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 5
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
	w.server.Serve(w.listener)
}

func (w *WormholeBoothElement) handleWorm(writer http.ResponseWriter, request *http.Request) {
	w.mainId.Log().Info("handleWorm")
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
	id, err := api.SpawnWormholeBooth(w.mainId.Cosmos(), api.WormholeBoothName, nil)
	if err != nil {
		return
	}
	w.mainId.AcceptWormhole(id, )

	if err = id.SetWormholeConn(w.mainId, conn); err != nil {
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

type WormholeBoothAtom struct {
	self     atomos.WormholeSelf
	conn     *websocket.Conn
}

func (w *WormholeBoothAtom) SpawnWormhole(self atomos.WormholeSelf, arg *api.WormholeBoothSpawnArg, data *api.WormholeBoothData) error {
	w.self = self
	w.self.Log().Info("Spawn")
	return nil
}

func (w *WormholeBoothAtom) SetWormholeConn(fromId atomos.Id, conn interface{}) error {
	w.self.Log().Info("SetWormholeConn")
	c, ok := conn.(*websocket.Conn)
	if !ok {
		return errors.New("set conn not websocket conn")
	}
	w.conn = c
	w.self.SetWormholeLooper(w.read)
	return nil
}

func (w *WormholeBoothAtom) read() error {
	messageType, bytes, err := w.conn.ReadMessage()
	if err != nil {
		w.conn.Close()
		w.conn = nil
		return err
	}
}

func (w *WormholeBoothAtom) close() {
	if w.conn
}

func (w *WormholeBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return nil
}

