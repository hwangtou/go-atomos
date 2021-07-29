package go_atomos

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/hwangtou/go-atomos/net/websocket"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	cosmosRemoteInitWait = 20 * time.Second
)

// Websocket Server

type cosmosRemotesHelper struct {
	self *CosmosSelf
	// Server
	server websocket.Server
}

func newCosmosRemoteHelper(s *CosmosSelf) *cosmosRemotesHelper {
	return &cosmosRemotesHelper{
		self:   s,
		server: websocket.Server{},
	}
}

type cosmosServerDelegate struct {
	helper *cosmosRemotesHelper
	*websocket.ServerDelegateBase
}

type cosmosServerConnDelegate struct {
	websocket.ServerConnDelegate
	remote *cosmosWatchRemote
}

func (c *cosmosServerConnDelegate) Connected() error {
	return c.remote.Connected()
}

func (c *cosmosServerConnDelegate) Receive(buf []byte) error {
	return c.remote.Receive(buf)
}

type cosmosClientDelegate struct {
	*websocket.ClientDelegateBase
	remote *cosmosWatchRemote
}

func (c *cosmosClientDelegate) Connected() error {
	return c.remote.Connected()
}

func (c *cosmosClientDelegate) Receive(buf []byte) error {
	return c.remote.Receive(buf)
}

// ServerDelegate Subclass

func (h *cosmosServerDelegate) NewServerConn(name string, conn *websocket.ServerConn) *websocket.ServerConn {
	conn.ServerConnDelegate = &cosmosServerConnDelegate{
		ServerConnDelegate: conn.ServerConnDelegate,
		remote: &cosmosWatchRemote{
			name:         name,
			helper:       h.helper,
			elements:     map[string]*ElementRemote{},
			initCh:       make(chan error, 1),
			isServerConn: true,
			ServerConn:   conn,
		},
	}
	return conn
}

func (h *cosmosServerDelegate) Started() {
}

func (h *cosmosServerDelegate) Stopped() {
	h.helper.self.logInfo("CosmosServer.Stopped: Stopping server, name=%s", h.helper.self.config.Node)
	for name, conn := range h.ServerDelegateBase.Conn {
		var err error
		h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, name=%s", name)
		switch c := conn.(type) {
		case *websocket.ServerConn:
			err = c.Conn.Stop()
		case *websocket.Client:
			err = c.Conn.Stop()
		}
		if err != nil {
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, name=%s,err=%v", name, err)
		}
	}
}

func (h *cosmosRemotesHelper) init() error {
	config := h.self.config
	delegate := &cosmosServerDelegate{
		helper: h,
		ServerDelegateBase: &websocket.ServerDelegateBase{
			Addr:     fmt.Sprintf(":%d", config.EnableRemoteServer.Port),
			Name:     config.Node,
			CertFile: config.EnableRemoteServer.CertPath,
			KeyFile:  config.EnableRemoteServer.KeyPath,
			Mux: map[string]func(http.ResponseWriter, *http.Request){
				"/atom_id":  h.handleAtomId,
				"/atom_msg": h.handleAtomMsg,
				"/":         h.handle404,
			},
			Logger: log.New(h, "", 0),
			Conn:   map[string]websocket.Connection{},
		},
	}
	if err := h.server.Init(delegate); err != nil {
		return err
	}
	h.server.Start()
	return nil
}

func (h *cosmosRemotesHelper) close() {
	h.server.Stop()
}

func (h *cosmosRemotesHelper) Write(l []byte) (int, error) {
	var msg string
	if len(l) > 0 {
		msg = string(l[:len(l)-1])
	}
	h.self.pushCosmosLog(LogLevel_Info, msg)
	return 0, nil
}

// Client

func (h *cosmosRemotesHelper) getOrConnectRemote(name, addr, certFile string) (*cosmosWatchRemote, error) {
	h.server.ServerDelegate.Lock()
	c, has := h.server.ServerDelegate.GetConn(name)
	if !has {
		r := &cosmosWatchRemote{
			name:         name,
			helper:       h,
			elements:     map[string]*ElementRemote{},
			initCh:       make(chan error, 1),
			isServerConn: false,
		}
		// Connect
		client := &websocket.Client{}
		delegate := &cosmosClientDelegate{
			remote:             r,
			ClientDelegateBase: websocket.NewClientDelegate(name, addr, certFile, h.server.ServerDelegate.GetLogger()),
		}
		client.Init(delegate)
		r.Client = client
		h.server.ServerDelegate.SetConn(name, r)
		h.server.ServerDelegate.Unlock()
		// Try connect if offline
		if err := client.Connect(); err != nil {
			h.server.ServerDelegate.GetLogger().Printf("cosmosRemotesHelper.getOrConnectRemote: Connect failed, err=%v", err)
			return nil, err
		}
		return r, nil
	} else {
		h.server.ServerDelegate.Unlock()
		return c.(*cosmosWatchRemote), nil
	}
}

func (h *cosmosRemotesHelper) getRemote(name string) *cosmosWatchRemote {
	h.server.ServerDelegate.Lock()
	defer h.server.ServerDelegate.Unlock()
	c, has := h.server.ServerDelegate.GetConn(name)
	if !has {
		return nil
	}
	return c.(*cosmosWatchRemote)
}

// Http Handlers.

// Refuse illegal request.
func (h *cosmosRemotesHelper) handle404(writer http.ResponseWriter, request *http.Request) {
	h.server.ServerDelegate.GetLogger().Printf("handle404, req=%+v", request)
	return
}

// Handle atom id.
func (h *cosmosRemotesHelper) handleAtomId(writer http.ResponseWriter, request *http.Request) {
	h.server.ServerDelegate.GetLogger().Printf("cosmosServer.handleAtomId")
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		h.server.ServerDelegate.GetLogger().Printf("cosmosServer.handleAtomId: Read body failed, err=%v", err)
		return
	}
	req, resp := &CosmosRemoteGetAtomIdReq{}, &CosmosRemoteGetAtomIdResp{}
	if err = proto.Unmarshal(body, req); err != nil {
		h.server.ServerDelegate.GetLogger().Printf("cosmosServer.handleAtomId: Unmarshal failed, err=%v", err)
		return
	}
	element, has := h.self.local.elements[req.Element]
	if has {
		_, has = element.atoms[req.Name]
		if has {
			resp.Has = true
		}
	}
	buf, err := proto.Marshal(resp)
	if err != nil {
		h.server.ServerDelegate.GetLogger().Printf("cosmosServer.handleAtomId: Marshal failed, err=%v", err)
		return
	}
	_, err = writer.Write(buf)
	if err != nil {
		h.server.ServerDelegate.GetLogger().Printf("cosmosServer.handleAtomId: Write failed, err=%v", err)
		return
	}
}

// Handle message connections.
func (h *cosmosRemotesHelper) handleAtomMsg(writer http.ResponseWriter, request *http.Request) {
	h.server.ServerDelegate.GetLogger().Printf("handleAtomMsg")
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		// TODO
		return
	}
	req := &CosmosRemoteMessagingReq{}
	resp := &CosmosRemoteMessagingResp{}
	if err = proto.Unmarshal(body, req); err != nil {
		// TODO
		return
	}
	arg, err := req.Args.UnmarshalNew()
	if err != nil {
		// TODO
		return
	}
	element, has := h.self.local.elements[req.To.Element]
	if has {
		a, has := element.atoms[req.To.Name]
		if has {
			resp.Has = true
			// TODO: Get from id
			fromId := &atomIdRemote{
				cosmosNode: nil,
				element:    nil,
				name:       "",
				version:    0,
				created:    time.Time{},
			}
			reply, err := h.self.local.MessageAtom(fromId, a, req.Message, arg)
			resp.Reply = MessageToAny(reply)
			if err != nil {
				resp.Error = err.Error()
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

// Cosmos Watch Remote

type cosmosWatchRemote struct {
	// Info
	name     string
	helper   *cosmosRemotesHelper
	mutex    sync.RWMutex
	elements map[string]*ElementRemote
	initCh   chan error
	// Watch
	isServerConn bool
	*websocket.ServerConn
	*websocket.Client

	// Requester
	requester *http.Client
}

func (r *cosmosWatchRemote) Receive(buf []byte) error {
	msg, err := r.decodeMessage(buf)
	if err != nil {
		return errors.New("receive error")
	}

	switch msg.MessageType {
	case CosmosWatchMessageType_Init:
		if r.isServerConn {
			r.name = msg.InitMessage.Config.NodeName
		} else {
			if r.name != msg.InitMessage.Config.NodeName {
				err = errors.New("cosmosRemote name not match")
				r.initCh <- err
				return err
			}
		}
		for name, elem := range msg.InitMessage.Config.Elements {
			ei, has := r.helper.self.local.elementInterfaces[name]
			if !has {
				r.helper.self.logFatal("cosmosRemote.watchConnReadAllInfo: Element not supported, element=%s", name)
				continue
			}
			r.elements[name] = &ElementRemote{
				ElementInterface: ei,
				cosmos:           r,
				name:             name,
				version:          elem.Version,
				cachedId:         map[string]*atomIdRemote{},
			}
		}
		r.initCh <- nil
	default:
		r.ServerConnDelegate.GetLogger().Printf("cosmosWatchRemote.Receive: Message type not supported, msg=%+v", msg)
	}
	return nil
}

func (r *cosmosWatchRemote) Connected() error {
	// Send local info.
	buf, err := r.encodeInitMessage()
	if err != nil {
		return err
	}
	if r.isServerConn {
		if err = r.ServerConn.Send(buf); err != nil {
			return err
		}
	} else {
		if err = r.Client.Send(buf); err != nil {
			return err
		}
	}

	// Wait for remote info.
	select {
	case err = <-r.initCh:
		if err != nil {
			return err
		}
	case <-time.After(cosmosRemoteInitWait):
		return errors.New("request timeout")
	}

	// Init http2 requester.
	tlsConfig, err := r.GetTLSConfig()
	if err != nil {
		return err
	}
	r.requester = &http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return nil
}

func (r *cosmosWatchRemote) Disconnected() {
}

// ServerConnDelegate

func (r *cosmosWatchRemote) ReconnectKickOld() {
}

func (r *cosmosWatchRemote) Reconnected() error {
	return r.Connected()
}

// ClientDelegate

func (r *cosmosWatchRemote) GetTLSConfig() (*tls.Config, error) {
	certPath := r.helper.self.config.EnableRemoteServer.CertPath
	if certPath == "" {
		return nil, nil
	}
	caCert, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{}
	if r.ShouldSkipVerify() {
		tlsConfig.InsecureSkipVerify = true
	} else {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		// Create TLS configuration with the certificate of the server.
		tlsConfig.RootCAs = caCertPool
	}
	return tlsConfig, nil
}

func (r *cosmosWatchRemote) ShouldSkipVerify() bool {
	return true
}

// ElementRemote

func (r *cosmosWatchRemote) getElement(name string) (*ElementRemote, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	elem, has := r.elements[name]
	if !has {
		err := fmt.Errorf(lCRGENotFound, name)
		r.helper.self.logFatal("%s", err.Error())
		return nil, err
	}
	return elem, nil
}

func (r *cosmosWatchRemote) setElement(name string, elem *ElementRemote) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, has := r.elements[name]; has {
		// Update.
		r.helper.self.logInfo(lCRSElementUpdate, name)
	} else {
		// Add.
		r.helper.self.logInfo(lCRSElementAdd, name)
	}
	r.elements[name] = elem
	return nil
}

func (r *cosmosWatchRemote) delElement(name string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check exists.
	_, has := r.elements[name]
	if !has {
		err := fmt.Errorf(lCRDElementNotFound, name)
		r.helper.self.logFatal("%s", err.Error())
		return err
	}
	// Try unload and unset.
	r.helper.self.logFatal(lCRDElementDel, name)
	delete(r.elements, name)
	return nil
}

// CosmosNode

func (r *cosmosWatchRemote) GetNodeName() string {
	return r.name
}

func (r *cosmosWatchRemote) IsLocal() bool {
	return false
}

func (r *cosmosWatchRemote) GetAtomId(elemName, atomName string) (Id, error) {
	// Get element.
	e, err := r.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atom.
	return e.GetAtomId(atomName)
}

// 暂时不支持从remote启动一个Atom。
func (r *cosmosWatchRemote) SpawnAtom(elemName, atomName string, arg proto.Message) (Id, error) {
	return nil, errors.New(lCRSAFailed)
}

func (r *cosmosWatchRemote) MessageAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (r *cosmosWatchRemote) KillAtom(fromId, toId Id) error {
	return toId.Element().KillAtom(fromId, toId)
}

// Coder

func (r *cosmosWatchRemote) decodeMessage(buf []byte) (*CosmosWatch, error) {
	msg := &CosmosWatch{}
	if err := proto.Unmarshal(buf, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *cosmosWatchRemote) encodeInitMessage() ([]byte, error) {
	config := r.helper.self.config
	msg := &CosmosWatch{
		MessageType: CosmosWatchMessageType_Init,
		InitMessage: &CosmosRemoteConnectInit{
			Config: &CosmosLocalConfig{
				EnableRemote: config.EnableRemoteServer != nil,
				NodeName:     config.Node,
				Elements:     map[string]*ElementConfig{},
			},
		},
	}
	if msg.InitMessage.Config.EnableRemote {
		for name, elem := range r.helper.self.local.elements {
			msg.InitMessage.Config.Elements[name] = &ElementConfig{
				Name:    name,
				Version: elem.current.ElementInterface.Config.Version,
			}
		}
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		r.ServerConnDelegate.GetLogger().Printf("CosmosClusterHelper.packInfo: Marshal failed, err=%v", err)
		return nil, err
	}
	return buf, nil
}

// Messaging

func (r *cosmosWatchRemote) request(uri string, buf []byte) (body []byte, err error) {
	reader := bytes.NewBuffer(buf)
	// Connect.
	var addr string
	if r.isServerConn {
		addr = r.ServerConnDelegate.GetAddr()
	} else {
		addr = r.ClientDelegate.GetAddr()
	}
	resp, err := r.requester.Post("https://"+addr+"/"+uri, "application/protobuf", reader)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
