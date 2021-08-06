package go_atomos

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/hwangtou/go-atomos/net/websocket"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
)

const (
	cosmosRemoteInitWait = 20 * time.Second
)

// Websocket Server

type cosmosServerConnDelegate struct {
	websocket.ServerConnDelegate
	remote *cosmosWatchRemote
}

func (c *cosmosServerConnDelegate) Connected() error {
	return c.remote.Connected()
}

func (c *cosmosServerConnDelegate) Reconnected() error {
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
	// Decode request message.
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		h.self.logError("cosmosRemotesHelper.handleAtomMsg: Cannot read request, body=%v,err=%v",
			request.Body, err)
		return
	}
	req := &CosmosRemoteMessagingReq{}
	resp := &CosmosRemoteMessagingResp{}
	if err = proto.Unmarshal(body, req); err != nil {
		h.self.logError("cosmosRemotesHelper.handleAtomMsg: Cannot unmarshal request, body=%v,err=%v",
			body, err)
		return
	}
	arg, err := req.Args.UnmarshalNew()
	if err != nil {
		// TODO
		return
	}
	// Get to atom.
	element, has := h.self.local.elements[req.To.Element]
	if has {
		a, has := element.atoms[req.To.Name]
		if has {
			// Got to atom.
			resp.Has = true
			// Get from atom, create if not exists.
			fromId := h.getFromId(req.From)
			// Messaging to atom.
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

func (h *cosmosRemotesHelper) getFromId(from *AtomId) (id *atomIdRemote) {
	remoteCosmos := h.self.remotes.getRemote(from.Node)
	if remoteCosmos == nil {
		return nil
	}
	remoteElement, err := remoteCosmos.getElement(from.Element)
	if remoteElement == nil || err != nil {
		return nil
	}
	return remoteElement.getOrCreateAtomId(from)
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
		for _, name := range msg.InitMessage.Config.Elements {
			ei, has := r.helper.self.local.interfaces[name]
			if !has {
				r.helper.self.logFatal("cosmosRemote.watchConnReadAllInfo: Element not supported, element=%s", name)
				continue
			}
			r.elements[name] = &ElementRemote{
				cosmos:    r,
				elemInter: ei,
				cachedId:  map[string]*atomIdRemote{},
			}
			r.helper.self.logInfo("cosmosRemote.watchConnReadAllInfo: Element added, element=%s", name)
		}
		r.initCh <- nil
	default:
		r.ServerConnDelegate.GetLogger().Printf("cosmosWatchRemote.Receive: Message type not supported, msg=%+v", msg)
	}
	return nil
}

func (r *cosmosWatchRemote) Connected() error {
	r.helper.self.logInfo("cosmosWatchRemote.Connected")
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
			r.helper.self.logInfo("cosmosWatchRemote.Connected: Init error, err=%v", err)
			return err
		}
		r.helper.self.logInfo("cosmosWatchRemote.Connected: Init succeed")
	case <-time.After(cosmosRemoteInitWait):
		r.helper.self.logInfo("cosmosWatchRemote.Connected: Init timeout")
		return errors.New("request timeout")
	}

	r.helper.self.logInfo("cosmosWatchRemote.Connected: Creating requester")
	// Init http2 requester.
	tlsConfig, err := r.GetTLSConfig()
	if err != nil {
		return err
	}
	r.requester = &http.Client{}
	if tlsConfig != nil {
		r.requester.Transport = &http2.Transport{
			TLSClientConfig: tlsConfig,
		}
	} else {
		// TODO: If it's not using certification, request will send via http1.1.
		r.requester.Transport = &http.Transport{}
		//r.requester.Transport = &http2.Transport{
		//	AllowHTTP: true,
		//	DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		//		return net.Dial(network, addr)
		//	},
		//}
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
	if r.helper.self.config.EnableCert == nil {
		return nil, nil
	}
	certPath := r.helper.self.config.EnableCert.CertPath
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
		err := fmt.Errorf("cosmosRemote.getElement: Element not found, nodeName=%s", name)
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
		r.helper.self.logInfo("cosmosRemote.setElement: Update exists element, nodeName=%s", name)
	} else {
		// Add.
		r.helper.self.logInfo("cosmosRemote.setElement: Add element, nodeName=%s", name)
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
		err := fmt.Errorf("cosmosRemote.delElement: Element not found, nodeName=%s", name)
		r.helper.self.logFatal("%s", err.Error())
		return err
	}
	// Try unload and unset.
	r.helper.self.logFatal("cosmosRemote.delElement: Delete element, nodeName=%s", name)
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
	return nil, errors.New("cosmosRemote.SpawnAtom: Cannot spawn remote atom")
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
				EnableRemote: config.EnableServer != nil,
				NodeName:     config.Node,
				Elements:     make([]string, 0, len(r.helper.self.local.elements)),
			},
		},
	}
	if msg.InitMessage.Config.EnableRemote {
		for name := range r.helper.self.local.elements {
			msg.InitMessage.Config.Elements = append(msg.InitMessage.Config.Elements, name)
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
	var schema, addr string
	if r.helper.self.config.EnableCert != nil {
		schema = "https://"
	} else {
		schema = "http://"
	}
	if r.isServerConn {
		addr = r.ServerConnDelegate.GetAddr()
	} else {
		addr = r.ClientDelegate.GetAddr()
	}
	resp, err := r.requester.Post(schema+addr+uri, "application/protobuf", reader)
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
