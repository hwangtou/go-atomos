package go_atomos

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

type Server struct {
	helper    *cosmosRemotesHelper
	server    *http.Server
	tlsConfig *tls.Config
	listener  net.Listener
	upgrade   websocket.Upgrader
}

const (
	WatchWsSchema  = "ws"
	WatchWssSchema = "wss"
	WatchUri       = "/watch"
	WatchHeader    = "name"
)

var (
	ErrHeaderInvalid = errors.New("websocket server accept header invalid")
)

func (s *Server) init(addr string, port int32) (err error) {
	// Server Mux.
	mux := http.NewServeMux()
	mux.HandleFunc(WatchUri, s.handleWatch)
	mux.HandleFunc(RemoteUriAtomId, s.handleAtomId)
	mux.HandleFunc(RemoteUriAtomMessage, s.handleAtomMsg)
	s.server = &http.Server{
		Addr:      fmt.Sprintf("%s:%d", addr, port),
		Handler:   mux,
		TLSConfig: s.tlsConfig,
		ErrorLog:  log.New(s.helper, "", 0),
	}
	if err = http2.ConfigureServer(s.server, &http2.Server{}); err != nil {
		s.helper.self.logError("Cosmos.Remote: Configure http2 failed, err=%v", err)
	}
	// Listen the local port.
	s.listener, err = net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) start() {
	if s.server.TLSConfig != nil {
		go s.server.ServeTLS(s.listener, "", "")
	} else {
		go s.server.Serve(s.listener)
	}
	s.helper.serverStarted()
}

func (s *Server) stop() {
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Close listener failed, err=%v", err)
		}
	}
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Close server failed, err=%v", err)
		}
	}
	s.helper.serverStopped()
}

func (s *Server) handleWatch(writer http.ResponseWriter, request *http.Request) {
	s.helper.self.logInfo("Cosmos.Remote: Watch Begin")
	// Check header.
	remoteName, err := s.acceptHeaderInfo(request.Header)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: Watch, Header remoteName invalid, err=%v", err)
		return
	}
	// Upgrade.
	wsConn, err := s.upgrade.Upgrade(writer, request, nil)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: Watch, Upgrade, err=%v", err)
		return
	}
	// Check duplicated.
	conn, hasOld := s.helper.newIncomingConn(remoteName, wsConn)
	// Accept new connection.
	if !hasOld {
		s.helper.self.logInfo("Cosmos.Remote: Watch, Accept new conn Begin")
		if err = conn.watch.initRequester(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Watch, Init requester failed, err=%v", err)
			if err = conn.Stop(); err != nil {
				s.helper.self.logError("Cosmos.Remote: Watch, Init requester conn stop error, err=%v", err)
			}
			s.helper.delConn(remoteName)
		}
		if err = s.acceptNewConnect(remoteName, conn); err != nil {
			s.helper.self.logError("Cosmos.Remote: Watch, Accept new conn failed, err=%v", err)
			if err = conn.Stop(); err != nil {
				s.helper.self.logError("Cosmos.Remote: Watch, Accept new conn stop error, err=%v", err)
			}
			s.helper.delConn(remoteName)
		}
		return
	}
	// Accept connection exist.
	if conn != nil {
		// Accepted connection reconnected.
		s.helper.self.logInfo("Cosmos.Remote: Watch, Accept reconnect Begin")
		if err = conn.watch.initRequester(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Watch, Init requester failed, err=%v", err)
			if err = conn.Stop(); err != nil {
				s.helper.self.logError("Cosmos.Remote: Watch, Init requester conn stop error, err=%v", err)
			}
			s.helper.delConn(remoteName)
		}
		if err = s.acceptReconnect(remoteName, conn, wsConn); err != nil {
			s.helper.self.logError("Cosmos.Remote: Watch, Accept reconnect failed, err=%v", err)
			if err = conn.Stop(); err != nil {
				s.helper.self.logError("Cosmos.Remote: Watch, Accept reconnect stop error, err=%v", err)
			}
			s.helper.delConn(remoteName)
			return
		}
		return
	}
	// Currently not supported, delete incoming conn.
	s.helper.self.logInfo("Cosmos.Remote: Watch, Currently not support server conn reconnect as client.")
	wsConn.Close()
	return
}

// Refuse illegal request.
func (s *Server) handle404(_ http.ResponseWriter, request *http.Request) {
	s.helper.self.logError("handle404, req=%+v", request)
	return
}

// Handle atom id.
func (s *Server) handleAtomId(writer http.ResponseWriter, request *http.Request) {
	//s.helper.self.logInfo("Cosmos.Remote: handleAtomId")
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomId, Read body failed, err=%v", err)
		return
	}
	req, resp := &CosmosRemoteGetAtomIdReq{}, &CosmosRemoteGetAtomIdResp{}
	if err = proto.Unmarshal(body, req); err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomId, Unmarshal failed, err=%v", err)
		return
	}
	element, has := s.helper.self.local.elements[req.Element]
	if has {
		a, err := element.getAtomId(req.Name)
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Has = a != nil
			resp.Error = ""
		}
	}
	buf, err := proto.Marshal(resp)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomId, Marshal failed, err=%v", err)
		return
	}
	_, err = writer.Write(buf)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomId, Write failed, err=%v", err)
		return
	}
}

// Handle message connections.
func (s *Server) handleAtomMsg(writer http.ResponseWriter, request *http.Request) {
	//s.helper.self.logInfo("handleAtomMsg")
	// Decode request message.
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomMsg, Cannot read request, body=%v,err=%v",
			request.Body, err)
		return
	}
	req := &CosmosRemoteMessagingReq{}
	resp := &CosmosRemoteMessagingResp{}
	if err = proto.Unmarshal(body, req); err != nil {
		s.helper.self.logError("Cosmos.Remote: handleAtomMsg, Cannot unmarshal request, body=%v,err=%v",
			body, err)
		return
	}
	arg, err := req.Args.UnmarshalNew()
	if err != nil {
		// TODO
		return
	}
	// Get to atom.
	element, has := s.helper.self.local.elements[req.To.Element]
	if has {
		a, err := element.GetAtomId(req.To.Name)
		if err != nil {
			resp.Error = err.Error()
		} else {
			if a != nil {
				// Got to atom.
				resp.Has = true
				// Get from atom, create if not exists.
				fromId := s.getFromId(req.From)
				// Messaging to atom.
				reply, err := s.helper.self.local.MessageAtom(fromId, a, req.Message, arg)
				resp.Reply = MessageToAny(reply)
				if err != nil {
					resp.Error = err.Error()
				}
			}
		}
	}
	buf, err := proto.Marshal(resp)
	if err != nil {
		// TODO
		return
	}
	_, err = writer.Write(buf)
	if err != nil {
		// TODO
		return
	}
}

// Http Handlers.

func (s *Server) getFromId(from *AtomId) Id {
	remoteCosmos := s.helper.self.remotes.getRemote(from.Node)
	if remoteCosmos == nil {
		return nil
	}
	remoteElement, err := remoteCosmos.getElement(from.Element)
	if remoteElement == nil || err != nil {
		return nil
	}
	return remoteElement.getOrCreateAtomId(from.Name)
}

func (s *Server) acceptHeaderInfo(header http.Header) (string, error) {
	// Check header.
	nodeName := header.Get(WatchHeader)
	if nodeName == "" {
		return "", ErrHeaderInvalid
	}
	return nodeName, nil
}

func (s *Server) acceptNewConnect(remoteName string, sc *incomingConn) error {
	sc.conn.run()

	s.helper.self.logInfo("Cosmos.Remote: AcceptNewConnect")
	if err := sc.Connected(sc); err != nil {
		if err := sc.Stop(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Stop accept error, err=%v", err)
		}
		return err
	}
	return nil
}

func (s *Server) acceptReconnect(info string, sc *incomingConn, conn *websocket.Conn) error {
	sc.conn.runningMutex.Lock()
	sc.conn.reconnecting = true
	sc.conn.runningMutex.Unlock()
	if err := sc.Stop(); err != nil {
		s.helper.self.logError("Cosmos.Remote: Accept reconnect stop old connect error, err=%v", err)
	} else {
		<-sc.conn.reconnectCh
	}

	sc.conn.conn = conn
	sc.conn.sender = make(chan *msg)
	sc.conn.run()

	s.helper.self.logInfo("Cosmos.Remote: AcceptReconnect")
	if err := sc.Reconnected(sc); err != nil {
		if err := sc.Stop(); err != nil {
			s.helper.self.logError("Cosmos.Remote: Accept reconnect error, err=%v", err)
		}
		return err
	}
	return nil
}
