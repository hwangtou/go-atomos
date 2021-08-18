package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

const (
	MainAtomName = "Main"
)

//
// Interface
//

// MainId is the interface of Greeter atomos.
//
type MainId interface {
	Id

	// Atom日志。
	// Atom Logs.
	Log() *atomLogsManager

	// Atom任务
	// Atom Tasks.
	Task() *atomTasksManager

	// Connect to remote CosmosNode.
	Connect(name, addr string) (CosmosNode, error)
}

// GreeterAtom is the atomos implements of Greeter atomos.
//
type MainAtom interface {
	Atom
	AtomSelf
}

// Main Element Develop

func newMainElement(self *CosmosSelf) *ElementLocal {
	m := &mainElement{
		self: self,
	}
	e := newElementLocal(self, &ElementImplementation{
		Developer:    m,
		Interface:    NewInterfaceFromDeveloper(m),
		AtomHandlers: map[string]MessageHandler{},
	})
	e.current.Interface.AtomSpawner = m.AtomSpawner
	return e
}

type mainElement struct {
	self *CosmosSelf
}

func newMainAtom(e *ElementLocal) *AtomCore {
	a := allocAtom()
	initAtom(a, e, MainAtomName, &mainAtom{
		self:     e.cosmos,
		AtomCore: a,
	})
	a.state = AtomSpawning
	e.atoms[MainAtomName] = a
	ac, _ := e.spawningAtom(a, nil)
	return ac
}

func (m *mainElement) Check() error {
	return nil
}

func (m *mainElement) Info() (name string, version uint64, logLevel LogLevel, initNum int) {
	return "Main", 1, m.self.config.LogLevel, 1
}

func (m *mainElement) Loaded(mainId Id) {
}

func (m *mainElement) Unloaded() {
}

func (m *mainElement) AtomConstructor() Atom {
	return &mainAtom{}
}

func (m *mainElement) AtomDataLoader(name string) (proto.Message, error) {
	return nil, nil
}

func (m *mainElement) AtomDataSaver(name string, data proto.Message) error {
	return nil
}

func (m *mainElement) AtomCanKill(id Id) bool {
	return true
}

func (m *mainElement) AtomSpawner(s AtomSelf, a Atom, arg, data proto.Message) error {
	return nil
}

// Main Atom

type mainAtom struct {
	self *CosmosSelf
	*AtomCore
}

func (m *mainAtom) Connect(name, addr string) (CosmosNode, error) {
	return m.self.Connect(name, addr)
}

func (m *mainAtom) Spawn(self AtomSelf, arg proto.Message) error {
	self.Log().Info("mainAtom.Spawn")
	return nil
}

func (m *mainAtom) Halt(from Id, cancels map[uint64]CancelledTask) proto.Message {
	m.CosmosSelf().logInfo("mainAtom.Halt: Start halting")
	return nil
}
