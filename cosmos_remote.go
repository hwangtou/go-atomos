package go_atomos

import (
	"bytes"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type CosmosRemote struct {
	sync.RWMutex
	main      *CosmosMain
	atomos    *BaseAtomos
	info      *CosmosRemoteConnectInfo
	available bool
	elements  map[string]*ElementRemote

	addr   string
	client *http.Client
}

func newCosmosRemote(main *CosmosMain, cosmos, addr string) *CosmosRemote {
	c := &CosmosRemote{
		RWMutex:   sync.RWMutex{},
		main:      main,
		atomos:    nil,
		info:      nil,
		available: false,
		elements:  map[string]*ElementRemote{},
		addr:      addr,
		client:    nil,
	}
	c.atomos = NewBaseAtomos(&IDInfo{
		Type:    IDType_Cosmos,
		Cosmos:  cosmos,
		Element: "",
		Atom:    "",
		Ext:     nil,
	}, main.runnable.config.LogLevel, c, c)
	if err := c.connect(cosmos); err != nil {
		c.main.Log().Fatal("Cosmos: Check remote connect failed. addr=(%s),err=(%v)", addr, err)
	}
	return c
}

// Implementation of ID

func (c *CosmosRemote) GetIDInfo() *IDInfo {
	if c == nil {
		return nil
	}
	return c.atomos.GetIDInfo()
}

func (c *CosmosRemote) String() string {
	return c.atomos.String()
}

func (c *CosmosRemote) Cosmos() CosmosNode {
	return c
}

func (c *CosmosRemote) Element() Element {
	return nil
}

func (c *CosmosRemote) GetName() string {
	return c.String()
}

func (c *CosmosRemote) State() AtomosState {
	return c.atomos.GetState()
}

func (c *CosmosRemote) IdleTime() time.Duration {
	return 0
}

func (c *CosmosRemote) MessageByName(from ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error) {
	return nil, NewError(ErrCosmosRemoteCannotMessage, "CosmosRemote: Cannot message remote.").AddStack(c.main)
}

func (c *CosmosRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosRemote) Kill(from ID, timeout time.Duration) *Error {
	return NewError(ErrCosmosRemoteCannotKill, "CosmosRemote: Cannot kill remote.").AddStack(c.main)
}

func (c *CosmosRemote) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewError(ErrCosmosRemoteCannotSendWormhole, "CosmosGlobal: Cannot send wormhole remote.").AddStack(c.main)
}

func (c *CosmosRemote) getCallChain() []ID {
	// TODO: Think about it.
	return nil
}

func (c *CosmosRemote) getElementLocal() *ElementLocal {
	return nil
}

func (c *CosmosRemote) getAtomLocal() *AtomLocal {
	return nil
}

func (c *CosmosRemote) getElementRemote() *ElementRemote {
	return nil
}

func (c *CosmosRemote) getAtomRemote() *AtomRemote {
	return nil
}

func (c *CosmosRemote) getIDTrackerManager() *IDTrackerManager {
	return nil
}

// Implementation of CosmosNode

func (c *CosmosRemote) GetNodeName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosRemote) CosmosIsLocal() bool {
	return false
}

func (c *CosmosRemote) CosmosGetElementID(elem string) (ID, *Error) {
	return c.getElement(elem)
}

func (c *CosmosRemote) CosmosGetElementAtomID(elem, name string) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(c.main)
	}
	return element.GetAtomID(name, NewIDTrackerInfo(3))
}

func (c *CosmosRemote) CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err
	}
	return element.SpawnAtom(name, arg, NewIDTrackerInfo(3))
}

func (c *CosmosRemote) CosmosMessageElement(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	return toID.Element().MessageElement(fromID, toID, message, timeout, args)
}

func (c *CosmosRemote) CosmosMessageAtom(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	return toID.Element().MessageAtom(fromID, toID, message, timeout, args)
}

func (c *CosmosRemote) CosmosScaleElementGetAtomID(fromID ID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return element.ScaleGetAtomID(fromID, message, timeout, args, NewIDTrackerInfo(3))
}

func (c *CosmosRemote) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *Error) {
	return nil, NewError(ErrCosmosRemoteCannotMessage, "CosmosRemote: Cannot send cosmos message.").AddStack(c.main)
}

func (c *CosmosRemote) OnScaling(from ID, name string, args proto.Message, tracker *IDTracker) (id ID, err *Error) {
	return nil, NewError(ErrCosmosRemoteCannotScale, "Cosmos: Cannot scale.").AddStack(c.main)
}

func (c *CosmosRemote) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	return NewError(ErrCosmosRemoteCannotSendWormhole, "Cosmos: Cannot send wormhole.").AddStack(c.main)
}

func (c *CosmosRemote) OnStopping(from ID, cancelled map[uint64]CancelledTask) *Error {
	return NewError(ErrCosmosRemoteCannotKill, "Cosmos: Cannot scale.").AddStack(c.main)
}

func (c *CosmosRemote) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	return false, nil
}

func (c *CosmosRemote) Spawn() {}

func (c *CosmosRemote) Set(message string) {}

func (c *CosmosRemote) Unset(message string) {}

func (c *CosmosRemote) Stopping() {}

func (c *CosmosRemote) Halted() {}

// Remote

func (c *CosmosRemote) getElement(name string) (*ElementRemote, *Error) {
	c.RLock()
	defer c.RUnlock()

	elem, has := c.elements[name]
	if !has {
		return nil, NewErrorf(ErrCosmosRemoteElementNotFound, "CosmosRemote: Element not found. name=(%s)", name).AddStack(c.main)
	}
	return elem, nil
}

func (c *CosmosRemote) connect(cosmos string) *Error {
	// Check available
	if c.available && c.client != nil {
		return nil
	}
	if c.client == nil {
		c.client = &http.Client{}
		c.client.Transport = &http2.Transport{
			TLSClientConfig: c.main.clientCert,
		}
	}

	// Request
	info := &CosmosRemoteConnectInfo{}
	if err := c.httpGet(c.addr+RemoteAtomosConnect, info, 0); err != nil {
		return err.AddStack(c.main)
	}
	if err := info.IsValid(cosmos); err != nil {
		return err.AddStack(c.main)
	}
	elements := make(map[string]*ElementRemote, len(info.Config.Elements))
	c.main.mutex.RLock()
	for name, elementRemoteInfo := range info.Config.Elements {
		e, has := c.main.elements[name]
		if !has {
			c.main.Log().Error("CosmosRemote: Connect info element not supported. name=(%s)", name)
			continue
		}
		elements[name] = newElementRemote(c, name, elementRemoteInfo, e.current)
	}
	c.main.mutex.RUnlock()
	c.info = info
	c.available = true
	c.elements = elements
	return nil
}

func (c *CosmosRemote) httpGet(addr string, response proto.Message, timeout time.Duration) *Error {
	rsp, er := c.client.Get(addr)
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP GET  failed. err=(%v)", er).AddStack(c.main)
	}
	defer rsp.Body.Close()
	rspBuf, er := ioutil.ReadAll(rsp.Body)
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP GET response failed. err=(%v)", er).AddStack(c.main)
	}
	er = proto.Unmarshal(rspBuf, response)
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP GET unmarshal response failed. err=(%v)", er).AddStack(c.main)
	}
	return nil
}

func (c *CosmosRemote) httpPost(addr string, request, response proto.Message, timeout time.Duration) *Error {
	reqBuf, er := proto.Marshal(request)
	if er != nil {
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP POST invalid request. err=(%v)", er).AddStack(c.main)
	}
	rsp, er := c.client.Post(addr, "application/protobuf", bytes.NewBuffer(reqBuf))
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP POST failed. err=(%v)", er).AddStack(c.main)
	}
	defer rsp.Body.Close()
	rspBuf, er := ioutil.ReadAll(rsp.Body)
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP POST response failed. err=(%v)", er).AddStack(c.main)
	}
	er = proto.Unmarshal(rspBuf, response)
	if er != nil {
		c.available = false
		return NewErrorf(ErrCosmosRemoteRequestFailed, "CosmosRemote: HTTP POST unmarshal response failed. err=(%v)", er).AddStack(c.main)
	}
	return nil
}
