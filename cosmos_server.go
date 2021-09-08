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

type cosmosWatchRemote struct {
	helper       *cosmosRemotesHelper
	delegate     ConnDelegate
	mutex        sync.RWMutex
	enableRemote *RemoteServerConfig
	elements     map[string]*ElementRemote
	initCh       chan error
	// Requester
	requester *http.Client
}

func newCosmosWatchRemote(helper *cosmosRemotesHelper, delegate ConnDelegate) *cosmosWatchRemote {
	return &cosmosWatchRemote{
		helper:    helper,
		delegate:  delegate,
		elements:  map[string]*ElementRemote{},
		initCh:    make(chan error, 1),
		requester: &http.Client{},
	}
}

func (r *cosmosWatchRemote) initRequester() error {
	r.helper.self.logInfo("cosmosWatchRemote.initRequester: Creating requester")
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

func (r *cosmosWatchRemote) Receive(conn ConnDelegate, buf []byte) error {
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
			ei, has := r.helper.self.local.interfaces[name]
			if !has {
				r.helper.self.logInfo("cosmosRemote.watchConnReadAllInfo: Element not supported, element=%s", name)
				ei = newPrivateElementInterface(name)
			}
			r.elements[name] = &ElementRemote{
				cosmos:    r,
				elemInter: ei,
				cachedId:  map[string]Id{},
			}
			r.helper.self.logInfo("cosmosRemote.watchConnReadAllInfo: Element added, element=%s", name)
		}
		r.initCh <- nil
	default:
		r.helper.self.logError("cosmosWatchRemote.Receive: Message type not supported, msg=%+v", msg)
	}
	return nil
}

func (r *cosmosWatchRemote) Connected(conn ConnDelegate) error {
	r.helper.self.logInfo("cosmosWatchRemote.Connected")
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
			r.helper.self.logInfo("cosmosWatchRemote.Connected: Init error, err=%v", err)
			return err
		}
		r.helper.self.logInfo("cosmosWatchRemote.Connected: Init succeed")
	case <-time.After(cosmosRemoteInitWait):
		r.helper.self.logInfo("cosmosWatchRemote.Connected: Init timeout")
		return errors.New("request timeout")
	}

	return nil
}

func (r *cosmosWatchRemote) Disconnected(conn ConnDelegate) {
}

// ServerConnDelegate

func (r *cosmosWatchRemote) ReconnectKickOld() {
}

func (r *cosmosWatchRemote) Reconnected(conn ConnDelegate) error {
	return r.Connected(conn)
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
	return r.helper.self.config.Node
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
				EnableRemote: config.EnableServer,
				NodeName:     config.Node,
				Elements:     make([]string, 0, len(r.helper.self.local.elements)),
			},
		},
	}
	for name := range r.helper.self.local.elements {
		msg.InitMessage.Config.Elements = append(msg.InitMessage.Config.Elements, name)
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		r.helper.self.logError("CosmosClusterHelper.packInfo: Marshal failed, err=%v", err)
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
