package go_atomos

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
)

const (
	cosmosRemoteInitWait = 20 * time.Second
)

// Cosmos Watch Remote
// TODO: Reconnect, EnableRemote

type cosmosRemote struct {
	helper       *cosmosRemoteServer
	delegate     ConnDelegate
	mutex        sync.RWMutex
	enableRemote *RemoteServerConfig
	elements     map[string]*ElementRemote
	initCh       chan error
	// Requester
	requester *http.Client
}

func newCosmosRemote(helper *cosmosRemoteServer, delegate ConnDelegate) *cosmosRemote {
	return &cosmosRemote{
		helper:    helper,
		delegate:  delegate,
		elements:  map[string]*ElementRemote{},
		initCh:    make(chan error, 1),
		requester: &http.Client{},
	}
}

func (r *cosmosRemote) initRequester() error {
	r.helper.self.logInfo("Remote.Init: Creating requester")
	// Init http2 requester.
	if r.helper.self.clientCert != nil {
		r.requester.Transport = &http2.Transport{
			TLSClientConfig: r.helper.self.clientCert,
		}
	} else {
		// TODO: If it's not using certification, request will send via http1.1.
		r.requester.Transport = &http.Transport{}
	}
	return nil
}

func (r *cosmosRemote) Receive(conn ConnDelegate, buf []byte) error {
	msg, err := r.decodeMessage(buf)
	if err != nil {
		return errors.New("receive error")
	}

	switch msg.MessageType {
	case CosmosWatchMessageType_Init:
		if msg.InitMessage == nil || msg.InitMessage.Config == nil {
			return errors.New("cosmosRemote config invalid")
		}
		switch c := conn.(type) {
		case *incomingConn:
			if c.name != msg.InitMessage.Config.NodeName {
				return errors.New("cosmosRemote name not match")
			}
		case *outgoingConn:
			if c.name != msg.InitMessage.Config.NodeName {
				err = errors.New("cosmosRemote name not match")
				r.initCh <- err
				return err
			}
		}
		r.enableRemote = msg.InitMessage.Config.EnableRemote
		for _, name := range msg.InitMessage.Config.Elements {
			ei, has := r.helper.self.runtime.runnable.interfaces[name]
			if !has {
				r.helper.self.logInfo("Remote.Conn: Read element not supported, element=%s", name)
				ei = newPrivateElementInterface(name)
			}
			r.elements[name] = &ElementRemote{
				cosmos:    r,
				elemInter: ei,
				cachedId:  map[string]*atomIdRemote{},
			}
			r.helper.self.logInfo("Remote.Conn: Read element added, element=%s", name)
		}
		r.initCh <- nil
	default:
		r.helper.self.logError("Remote.Conn: Message type not supported, msg=%+v", msg)
	}
	return nil
}

func (r *cosmosRemote) Connected(conn ConnDelegate) error {
	r.helper.self.logInfo("Remote.Conn: Connected")
	// Send local info.
	buf, err := r.encodeInitMessage()
	if err != nil {
		return err
	}
	switch c := conn.(type) {
	case *incomingConn:
		if err = c.conn.Send(buf); err != nil {
			return err
		}
	case *outgoingConn:
		if err = c.conn.Send(buf); err != nil {
			return err
		}
	}

	// Wait for remote info.
	select {
	case err = <-r.initCh:
		if err != nil {
			r.helper.self.logInfo("Remote.Conn: Init error, err=%v", err)
			return err
		}
		r.helper.self.logInfo("Remote.Conn: Init succeed")
	case <-time.After(cosmosRemoteInitWait):
		r.helper.self.logInfo("Remote.Conn: Init timeout")
		return errors.New("request timeout")
	}

	return nil
}

func (r *cosmosRemote) Disconnected(conn ConnDelegate) {
}

// ServerConnDelegate

func (r *cosmosRemote) ReconnectKickOld() {
}

func (r *cosmosRemote) Reconnected(conn ConnDelegate) error {
	return r.Connected(conn)
}

// ElementRemote

func (r *cosmosRemote) getElement(name string) (*ElementRemote, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	elem, has := r.elements[name]
	if !has {
		err := fmt.Errorf("Remote.Element: Element not found, nodeName=%s", name)
		r.helper.self.logFatal("%s", err.Error())
		return nil, err
	}
	return elem, nil
}

func (r *cosmosRemote) setElement(name string, elem *ElementRemote) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, has := r.elements[name]; has {
		// Update.
		r.helper.self.logInfo("Remote.Element: Update exists element, nodeName=%s", name)
	} else {
		// Add.
		r.helper.self.logInfo("Remote.Element: Add element, nodeName=%s", name)
	}
	r.elements[name] = elem
	return nil
}

func (r *cosmosRemote) delElement(name string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check exists.
	_, has := r.elements[name]
	if !has {
		err := fmt.Errorf("Remote.Element: Element not found, nodeName=%s", name)
		r.helper.self.logFatal("%s", err.Error())
		return err
	}
	// Try unload and unset.
	r.helper.self.logFatal("Remote.Element: Delete element, nodeName=%s", name)
	delete(r.elements, name)
	return nil
}

// CosmosNode

func (r *cosmosRemote) GetNodeName() string {
	return r.helper.self.config.Node
}

func (r *cosmosRemote) IsLocal() bool {
	return false
}

func (r *cosmosRemote) GetAtomId(elemName, atomName string) (Id, error) {
	// Get element.
	e, err := r.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atomos.
	return e.GetAtomId(atomName)
}

// 暂时不支持从remote启动一个Atom。
func (r *cosmosRemote) SpawnAtom(elemName, atomName string, arg proto.Message) (Id, error) {
	return nil, errors.New("Remote.SpawnAtom: Cannot spawn remote atomos")
}

func (r *cosmosRemote) MessageAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (r *cosmosRemote) KillAtom(fromId, toId Id) error {
	return toId.Element().KillAtom(fromId, toId)
}

// Coder

func (r *cosmosRemote) decodeMessage(buf []byte) (*CosmosWatch, error) {
	msg := &CosmosWatch{}
	if err := proto.Unmarshal(buf, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *cosmosRemote) encodeInitMessage() ([]byte, error) {
	config := r.helper.self.config
	msg := &CosmosWatch{
		MessageType: CosmosWatchMessageType_Init,
		InitMessage: &CosmosRemoteConnectInit{
			Config: &CosmosLocalConfig{
				EnableRemote: config.EnableServer,
				NodeName:     config.Node,
				Elements:     make([]string, 0, len(r.helper.self.runtime.elements)),
			},
		},
	}
	for name := range r.helper.self.runtime.elements {
		msg.InitMessage.Config.Elements = append(msg.InitMessage.Config.Elements, name)
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		r.helper.self.logError("Remote.Conn: Marshal failed, err=%v", err)
		return nil, err
	}
	return buf, nil
}

// Messaging

func (r *cosmosRemote) request(uri string, buf []byte) (body []byte, err error) {
	reader := bytes.NewBuffer(buf)
	// Connect.
	var schema, addr string
	if r.helper.self.config.EnableCert != nil {
		schema = "https://"
	} else {
		schema = "http://"
	}
	addr = r.delegate.GetAddr()
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
