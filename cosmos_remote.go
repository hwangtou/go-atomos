package go_atomos

import (
	"bytes"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
	"time"
)

type CosmosRemote struct {
	main *CosmosMain

	addr   string
	client *http.Client

	info  *CosmosRemoteInfo
	state CosmosRemoteState

	// Elements
	elements map[string]*ElementRemote
	//mutex    sync.RWMutex
}

const (
	CosmosRemoteNotConnected CosmosRemoteState = 0
	CosmosRemoteConnected    CosmosRemoteState = 1
)

type CosmosRemoteState int

func newCosmosRemote(main *CosmosMain, addr string) *CosmosRemote {
	return &CosmosRemote{
		main:   main,
		addr:   addr,
		client: nil,
		info:   nil,
		state:  CosmosRemoteNotConnected,
	}
}

func (c *CosmosRemote) httpGet(addr string, response proto.Message) *ErrorInfo {
	rsp, er := c.client.Get(addr)
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "get failed, err=(%v)", er).AutoStack(nil, nil)
	}
	defer rsp.Body.Close()
	rspBuf, er := ioutil.ReadAll(rsp.Body)
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "get response failed, err=(%v)", er).AutoStack(nil, nil)
	}
	er = proto.Unmarshal(rspBuf, response)
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "unmarshal response failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}

func (c *CosmosRemote) httpPost(addr string, request, response proto.Message) *ErrorInfo {
	reqBuf, er := proto.Marshal(request)
	if er != nil {
		return NewErrorf(ErrAtomMessageArgType, "invalid request, err=(%v)", er).AutoStack(nil, nil)
	}
	rsp, er := c.client.Post(addr, "application/protobuf", bytes.NewBuffer(reqBuf))
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "get failed, err=(%v)", er).AutoStack(nil, nil)
	}
	defer rsp.Body.Close()
	rspBuf, er := ioutil.ReadAll(rsp.Body)
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "get response failed, err=(%v)", er).AutoStack(nil, nil)
	}
	er = proto.Unmarshal(rspBuf, response)
	if er != nil {
		c.state = CosmosRemoteNotConnected
		return NewErrorf(ErrRemoteConnectFailed, "unmarshal response failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}

func (c *CosmosRemote) connect() *ErrorInfo {
	if c.client != nil && c.state == CosmosRemoteConnected {
		return nil
	}
	if c.client == nil {
		c.client = &http.Client{}
		c.client.Transport = &http2.Transport{
			TLSClientConfig: c.main.clientCert,
		}
	}
	info := &CosmosRemoteInfo{}
	if err := c.httpGet(c.addr+RemoteAtomosConnect, info); err != nil {
		return err.AutoStack(nil, nil)
	}
	elements := make(map[string]*ElementRemote, len(info.Elements))
	for name, elementRemoteInfo := range info.Elements {
		e, has := c.main.elements[name]
		if !has {
			// TODO
			continue
		}
		elements[name] = newElementRemote(c, name, elementRemoteInfo, e.current.Interface)
	}
	c.info = info
	c.elements = elements
	c.state = CosmosRemoteConnected
	return nil
}

// Implementation of ID

func (c *CosmosRemote) GetIDInfo() *IDInfo {
	if c == nil || c.info == nil {
		return nil
	}
	return c.info.Id
}

func (c *CosmosRemote) String() string {
	return c.GetIDInfo().Info()
}

func (c *CosmosRemote) Release() {
	//if c.state == CosmosRemoteConnected || c.client != nil {
	//	c.client = nil
	//	c.info = nil
	//	c.state = CosmosRemoteNotConnected
	//}
}

func (c *CosmosRemote) Cosmos() CosmosNode {
	return c
}

func (c *CosmosRemote) Element() Element {
	return nil
}

func (c *CosmosRemote) GetName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosRemote) State() AtomosState {
	panic("not supported")
}

func (c *CosmosRemote) IdleDuration() time.Duration {
	panic("not supported")
}

func (c *CosmosRemote) MessageByName(from ID, name string, in proto.Message) (proto.Message, *ErrorInfo) {
	return nil, NewError(ErrMainCannotMessage, "Cannot message main")
}

func (c *CosmosRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosRemote) Kill(from ID) *ErrorInfo {
	return NewError(ErrMainCannotKill, "Cannot kill main")
}

func (c *CosmosRemote) SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	panic("not supported")
}

func (c *CosmosRemote) getCallChain() []ID {
	panic("not supported")
}

func (c *CosmosRemote) getElementLocal() *ElementLocal {
	panic("not supported")
}

func (c *CosmosRemote) getAtomLocal() *AtomLocal {
	panic("not supported")
}

func (c *CosmosRemote) getElementRemote() *ElementRemote {
	panic("not supported")
}

func (c *CosmosRemote) getAtomRemote() *AtomRemote {
	panic("not supported")
}

// Implementation of CosmosNode

func (c *CosmosRemote) GetNodeName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosRemote) CosmosIsLocal() bool {
	return false
}

func (c *CosmosRemote) CosmosGetElementID(elem string) (ID, *ErrorInfo) {
	return c.getElement(elem)
}

func (c *CosmosRemote) CosmosGetElementAtomID(elem, name string) (ID, *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.GetAtomID(name)
}

func (c *CosmosRemote) CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *ErrorInfo) {
	return nil, NewErrorf(ErrRemoteCannotRemoteSpawn, "cannot spawn remote atom").AutoStack(nil, nil)
}

func (c *CosmosRemote) CosmosMessageElement(fromID, toID ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return toID.Element().MessageElement(fromID, toID, message, args)
}

func (c *CosmosRemote) CosmosMessageAtom(fromID, toID ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return toID.Element().MessageAtom(fromID, toID, message, args)
}

func (c *CosmosRemote) CosmosScaleElementGetAtomID(fromID ID, elem, message string, args proto.Message) (ID ID, err *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.ScaleGetAtomID(fromID, message, args)
}

// Element Inner

func (c *CosmosRemote) getElement(name string) (*ElementRemote, *ErrorInfo) {
	if c.info == nil {
		return nil, NewErrorf(ErrRemoteNotConnect, "not connected")
	}
	elem, has := c.elements[name]
	if !has {
		return nil, NewErrorf(ErrMainElementNotFound, "Main: Local element not found, name=(%s)", name)
	}
	return elem, nil
}

//func (c *CosmosRemote) update(token string) {
//
//}
