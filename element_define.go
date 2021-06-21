package go_atomos

import "google.golang.org/protobuf/proto"

type ElementDefine struct {
	Config *ElementConfig
	AtomIdFactory ElementIdFactory
	AtomCreator   ElementAtomNewer
	AtomSaver     ElementAtomSaver
	AtomCanKill   ElementAtomCanKill
	AtomCalls     map[string]*ElementAtomCall
}

type ElementLordScheduler func(CosmosClusterHelper)
type ElementIdFactory func(c CosmosNode, atomName string) (Id, error)
type ElementAtomNewer func() Atom
type ElementAtomSaver func(Id, AtomStateful) error
type ElementAtomCanKill func(Id) bool

type ElementImplement interface {
	//Check() error
	Info() (name string, version uint64, logLevel LogLevel, initNum int)
	LordScheduler(CosmosClusterHelper)
	//AtomIdFactory(*CosmosSelf, string, *AtomCore) Id
	AtomCreator() Atom
	AtomSaver(Id, AtomStateful) error
	// Id is info which Id would like to kill the target AtomCore.
	AtomCanKill(Id) bool
	//AtomCalls() map[string]*ElementAtomCall
}

//type ElementImplementBase struct {
//	Name        string
//	Version     uint64
//	LogLevel    LogLevel
//	AtomInitNum int
//}

// ElementDefine Message
type ElementAtomCall struct {
	Handler MessageHandler
	InDec   MessageDecoder
	OutDec  MessageDecoder
}

type MessageHandler func(from Id, to Atom, in proto.Message) (out proto.Message, err error)
type MessageDecoder func(buf []byte) (proto.Message, error)
