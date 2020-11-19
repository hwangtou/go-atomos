package atomos

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"log"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc/mvccpb"
	"tcpacket"
)

var ErrDiffCosmos = errors.New("atomos: Different cosmos")
var ErrDiffNode = errors.New("atomos: Different node")
var ErrIllegal = errors.New("atomos: Illegal")
var ErrPacket = errors.New("atomos: Packet")
var ErrArgCodec = errors.New("atomos: Arg codec")
var ErrSessionId = errors.New("atomos: Session ID")
var ErrClosingRemote = errors.New("atomos: ClosingRemote")

type nodes struct {
	etcd
	cosmos   *Cosmos
	hub      *tcpacket.Hub
	toUpdate bool
	local    *CosmosData
	remote   map[string]*remoteNode
	sessions sessionsManager
}

type etcd struct {
	client  *clientv3.Client
	watcher clientv3.Watcher
	context context.Context
	cancel  context.CancelFunc
}

func (n *nodes) cosmosPrefix() string {
	return fmt.Sprintf("/cosmos/%s/node/", n.cosmos.cfg.CosmosId)
}

func (n *nodes) init(c *Cosmos, typeDesc ...*AtomTypeDesc) (err error) {
	n.cosmos = c
	n.remote = map[string]*remoteNode{}
	n.sessions = sessionsManager{
		sessions:  map[uint64]*sessionReply{},
		sessionId: 0,
	}
	n.context, n.cancel = context.WithCancel(context.Background())
	n.cosmos.url = fmt.Sprintf("%s%s", n.cosmosPrefix(), n.cosmos.cfg.Node)
	if len(c.cfg.EtcdEndpoints) == 0 {
		c.cfg.EtcdEndpoints = []string{
			"http://127.0.0.1:2379",
		}
	}
	n.client, err = clientv3.New(clientv3.Config{
		Endpoints:   c.cfg.EtcdEndpoints,
		DialTimeout: time.Duration(c.cfg.EtcdDialTimeout) * time.Second,
		Username:    c.cfg.EtcdUsername,
		Password:    c.cfg.EtcdPassword,
	})
	if err != nil {
		return err
	}
	for _, desc := range typeDesc {
		n.cosmos.ats.addType(desc)
	}
	// Listen
	n.hub, err = tcpacket.NewHub(tcpacket.Config{
		HeaderSize: n.cosmos.cfg.ConnHeadSize,
		Network:    n.cosmos.cfg.ListenNetwork,
		Address:    n.cosmos.cfg.ListenAddress,
		HeartBeat:  n.cosmos.cfg.ConnHeartBeat,
	}, n.acceptedConn)
	if err != nil {
		return err
	}
	go n.hub.Listen()
	// Etcd cluster
	if err = n.etcdDaemonNode(n.cosmos.url, n.cosmos.cfg.EtcdTTL); err != nil {
		return err
	}
	if err = n.etcdWatchNodes(n.cosmosPrefix()); err != nil {
		return err
	}
	return nil
}

// Conn

// In coming conn

func (n *nodes) acceptedConn(writer tcpacket.Writer) (tcpacket.Reader, error) {
	log.Println("acceptedConn")
	return &remoteConn{n, writer}, nil
}

type remoteConn struct {
	*nodes
	tcpacket.Writer
}

func (r *remoteConn) RecvPacket(nonCopiedBuf []byte) {
	log.Println("remoteConn.RecvPacket")
	in := &RemotePacket{}
	err := proto.Unmarshal(nonCopiedBuf, in)
	if err != nil {
		log.Println(err)
		return
	}
	go func() {
		var err error
		out := &RemotePacket{}
		switch in.Type {
		case RemotePacket_GetAtomReq:
			err = r.handleGetAtomReq(in, out)
		case RemotePacket_CallAtomReq:
			err = r.handleCallAtomReq(in, out)
		default:
			err = ErrIllegal
		}
		if err != nil {
			log.Println(err)
			out.Error = err.Error()
		}
		buf, err := proto.Marshal(out)
		if err != nil {
			log.Println(err)
			return
		}
		if err = r.SendPacket(buf); err != nil {
			log.Println(err)
		}
	}()
}

func (r *remoteConn) handleGetAtomReq(in, out *RemotePacket) error {
	out.SessionId = in.SessionId
	out.Type = RemotePacket_GetAtomResp
	out.GetAtomResp = &RemoteGetAtomResponse{}
	if in == nil || in.GetAtomReq == nil {
		out.Error = ErrPacket.Error()
		return ErrPacket
	}
	req, resp := in.GetAtomReq, out.GetAtomResp
	if req.ToNode != r.nodes.cosmos.cfg.Node {
		return ErrDiffNode
	}
	t := r.cosmos.ats.getType(req.AtomType)
	if t == nil {
		return ErrAtomTypeNotExists
	}
	_, has := t.atoms[req.AtomName]
	resp.Has = has
	return nil
}

func (r *remoteConn) handleCallAtomReq(in, out *RemotePacket) error {
	out.SessionId = in.SessionId
	out.Type = RemotePacket_CallAtomResp
	out.CallAtomResp = &RemoteCallAtomResponse{}
	if in == nil || in.CallAtomReq == nil {
		out.Error = ErrPacket.Error()
		return ErrPacket
	}
	req, resp := in.CallAtomReq, out.CallAtomResp
	if req.ToNode != r.nodes.cosmos.cfg.Node {
		return ErrDiffNode
	}
	t := r.cosmos.ats.getType(req.AtomType)
	if t == nil {
		return ErrAtomTypeNotExists
	}
	aw, has := t.atoms[req.AtomName]
	if !has {
		return ErrAtomNameNotExists
	}
	c, has := t.calls[req.CallName]
	log.Println("reqCallName", req.CallName, "aType.calls", t.calls)
	if !has {
		return ErrAtomCallNotExists
	}
	pb, err := c.ArgDec(req.CallArgs)
	if err != nil {
		return ErrArgCodec
	}
	res, err := c.Func(aw.aIns, pb)
	if err != nil {
		resp.Error = err.Error()
	}
	buf, err := proto.Marshal(res)
	if err != nil {
		return ErrPacket
	}
	resp.CallReply = buf
	return nil
}

func (r *remoteConn) RecvClose(err error) {
	log.Println("remoteConn.RecvClose")
	// todo
}

// Out going conn

func (n *nodes) outgoingConn(node string, a *CosmosData) {
	log.Println("outgoingConn")
	if old, has := n.remote[node]; has {
		if old.Addr == a.Addr {
			n.remote[node] = &remoteNode{
				nodes:      n,
				Writer:     old.Writer,
				CosmosData: a,
			}
			return
		}
		old.Close()
		delete(n.remote, node)
	}
	var err error
	conn := &remoteNode{
		nodes:      n,
		CosmosData: a,
	}
	conn.Writer, err = n.hub.Dial(a.Network, a.Addr, conn)
	if err != nil {
		log.Println("dialing", err)
		return
	}
	n.remote[node] = conn
}

type remoteNode struct {
	*nodes
	tcpacket.Writer
	*CosmosData
}

func (r *remoteNode) CloseAtom(aType, aName string) error {
	return ErrClosingRemote
}

func (r *remoteNode) RecvPacket(nonCopiedBuf []byte) {
	log.Println("remoteNode.RecvPacket")
	in := &RemotePacket{}
	err := proto.Unmarshal(nonCopiedBuf, in)
	if err != nil {
		log.Println(err)
		return
	}
	go func() {
		var err error
		reply, has := r.nodes.sessions.sessions[in.SessionId]
		if !has {
			log.Println(ErrSessionId)
			return
		}
		delete(r.nodes.sessions.sessions, in.SessionId)
		if in.Error != "" {
			log.Println(in.Error)
			return
		}
		switch in.Type {
		case RemotePacket_GetAtomResp:
			err = r.handleGetAtomResp(in, reply)
		case RemotePacket_CallAtomResp:
			err = r.handleCallAtomResp(in, reply)
		default:
			err = ErrIllegal
		}
		if err != nil {
			log.Println(err)
		}
	}()
}

func (r *remoteNode) handleGetAtomResp(in *RemotePacket, reply *sessionReply) error {
	reply.p = in
	reply.wait <- struct{}{}
	return nil
}

func (r *remoteNode) handleCallAtomResp(in *RemotePacket, reply *sessionReply) error {
	reply.p = in
	reply.wait <- struct{}{}
	return nil
}

func (r *remoteNode) RecvClose(err error) {
	log.Println("remoteNode.RecvClose")
	// todo
}

func (r *remoteNode) sendGetAtomReq(node, aType, aName string) (bool, error) {
	sId, reply := r.nodes.sessions.newReply()
	out := &RemotePacket{
		SessionId: sId,
		Type:      RemotePacket_GetAtomReq,
		GetAtomReq: &RemoteGetAtomRequest{
			FromNode: r.nodes.cosmos.cfg.Node,
			ToNode:   node,
			AtomType: aType,
			AtomName: aName,
		},
	}
	buf, err := proto.Marshal(out)
	if err != nil {
		log.Println(err)
		return false, err
	}
	log.Println("remoteNode.sendGetAtomReq a")
	err = r.SendPacket(buf)
	_ = <-reply.wait
	log.Println("remoteNode.sendGetAtomReq b")
	if reply.p == nil || reply.p.GetAtomResp == nil {
		return false, ErrIllegal
	}
	return reply.p.GetAtomResp.Has, nil
}

func (r *remoteNode) sendCallAtomReq(node, aType, aName, cName string, arg proto.Message, dec CallReplyDecode) (proto.Message, error) {
	sId, reply := r.nodes.sessions.newReply()
	ab, err := proto.Marshal(arg)
	if err != nil {
		return nil, err
	}
	out := &RemotePacket{
		SessionId: sId,
		Type:      RemotePacket_CallAtomReq,
		CallAtomReq: &RemoteCallAtomRequest{
			FromNode: r.nodes.cosmos.cfg.Node,
			ToNode:   node,
			AtomType: aType,
			AtomName: aName,
			CallName: cName,
			CallArgs: ab,
		},
	}
	buf, err := proto.Marshal(out)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	err = r.SendPacket(buf)
	_ = <-reply.wait
	if reply.p == nil || reply.p.CallAtomResp == nil || reply.p.CallAtomResp.CallReply == nil {
		return nil, ErrIllegal
	}
	return dec(reply.p.CallAtomResp.CallReply)
}

// Etcd

func (n *nodes) etcdDaemonNode(url string, ttl int64) error {
	log.Println(url)
	// Check exists
	if err := n.etcdRegisterNode(url, ttl); err != nil {
		return err
	}
	// Keep alive
	go func() {
		ticker := time.NewTicker(time.Duration(ttl) * time.Second)
		for {
			if err := n.etcdRegisterNode(url, ttl); err != nil {
				log.Println("atomos: Daemon err")
			}
			<-ticker.C
		}
	}()
	return nil
}

func (n *nodes) etcdRegisterNode(url string, ttl int64) error {
	getResp, err := n.client.Get(context.Background(), url)
	if err != nil {
		return err
	}
	if getResp.Count == 0 || n.toUpdate {
		n.toUpdate = false
		m := jsonpb.Marshaler{}
		value, err := m.MarshalToString(n.cosmos.getCosmosData())
		if err != nil {
			return err
		}
		leaseResp, err := n.client.Grant(context.Background(), ttl)
		if err != nil {
			return err
		}
		_, err = n.client.Put(context.Background(), url, value, clientv3.WithLease(leaseResp.ID))
		if err != nil {
			return err
		}
		keepCh, err := n.client.KeepAlive(context.Background(), leaseResp.ID)
		if err != nil {
			return err
		}
		go func() {
			for {
				_, more := <-keepCh
				if !more {
					log.Println("keepCh stop")
					return
				}
			}
		}()
	}
	return nil
}

func (n *nodes) etcdUnregisterNode(url string) {
	if n.client != nil {
		if _, err := n.client.Delete(context.Background(), url); err != nil {
			log.Println(err)
		}
	}
}

func (n *nodes) etcdWatchNodes(urlPrefix string) error {
	// get exists nodes
	getResp, err := n.client.Get(context.Background(), n.cosmosPrefix(), clientv3.WithPrefix())
	if err != nil {
		return err
	}
	for _, kv := range getResp.Kvs {
		a := &CosmosData{}
		node := strings.TrimPrefix(string(kv.Key), n.cosmosPrefix())
		if err := jsonpb.UnmarshalString(string(kv.Value), a); err != nil {
			log.Println("a01", err)
			continue
		}
		if a.CosmosId != n.cosmos.cfg.CosmosId {
			log.Println("a02", ErrDiffCosmos, a.CosmosId, n.cosmos.cfg.CosmosId)
			continue
		}
		if a.Node == n.cosmos.cfg.Node || a.Node != node {
			log.Println("a03")
			continue
		}
		n.outgoingConn(node, a)
	}
	// watch
	n.watcher = clientv3.NewWatcher(n.client)
	watchCh := n.watcher.Watch(n.context, n.cosmosPrefix(), clientv3.WithPrefix())
	// watch
	go func() {
		for resp := range watchCh {
			for _, ev := range resp.Events {
				node := strings.TrimPrefix(string(ev.Kv.Key), n.cosmosPrefix())
				log.Printf("(%s) %s %q=%q [%s]\n", node, ev.Type, ev.Kv.Key, ev.Kv.Value, ev.IsModify())
				// todo lock
				switch ev.Type {
				case mvccpb.PUT:
					a := &CosmosData{}
					if err := jsonpb.UnmarshalString(string(ev.Kv.Value), a); err != nil {
						log.Println("a1", err)
						continue
					}
					if a.CosmosId != n.cosmos.cfg.CosmosId {
						log.Println("a2", ErrDiffCosmos, a.CosmosId, n.cosmos.cfg.CosmosId)
						continue
					}
					if a.Node == n.cosmos.cfg.Node || a.Node != node {
						continue
					}
					n.outgoingConn(node, a)
				case mvccpb.DELETE:
					if old, has := n.remote[node]; has {
						old.Close()
						delete(n.remote, node)
					}
					log.Println("closing")
				}
			}
		}
	}()
	return nil
}

// Session

type sessionsManager struct {
	sync.Mutex
	sessions  map[uint64]*sessionReply
	sessionId uint64
}

type sessionReply struct {
	p    *RemotePacket
	wait chan struct{}
}

func (s *sessionsManager) newReply() (uint64, *sessionReply) {
	s.Mutex.Lock()
	for {
		s.sessionId += 1
		_, has := s.sessions[s.sessionId]
		if !has {
			break
		}
		log.Println("session")
	}
	reply := &sessionReply{
		wait: make(chan struct{}),
	}
	s.sessions[s.sessionId] = reply
	s.Mutex.Unlock()
	return s.sessionId, reply
}
