package go_atomos

//
//import (
//	"errors"
//	"google.golang.org/protobuf/proto"
//)
//
//const (
//	MainAtomName = "Main"
//)
//
////
//// Interface
////
//
//// MainId is the interface of Greeter atomos.
////
//type MainId interface {
//	//ID
//
//	// Log
//	// Atom日志。
//	// Atom Logs.
//	Log() AtomosLogging
//
//	// Task
//	// Atom任务
//	// Atom Tasks.
//	Task() AtomosTasking
//
//	//// Connect to remote CosmosNode.
//	//Connect(name, addr string) (CosmosNode, error)
//
//	//// Config
//	//// Clone of Config
//	//Config() *Config
//	//
//	//// CustomizeConfig
//	//// Get Customize Config.
//	//CustomizeConfig(name string) (string, error)
//}
//
//// GreeterAtom is the atomos implements of Greeter atomos.
////
//type MainAtom interface {
//	Atom
//	AtomSelf
//}
//
//// Main Element Develop
//
//func newMainElement(self *CosmosSelf) *ElementLocal {
//	m := &mainElement{
//		self: self,
//	}
//	e := newElementLocal(self, 1)
//	e.current = &ElementImplementation{
//		Developer: m,
//		Interface: &ElementInterface{
//			Name: MainAtomName,
//			Config: &ElementConfig{
//				Name:        MainAtomName,
//				Version:     1,
//				LogLevel:    self.config.LogLevel,
//				AtomInitNum: 1,
//				Messages:    map[string]*AtomMessageConfig{},
//			},
//			AtomSpawner: m.AtomSpawner,
//			AtomIdConstructor: func(id Id) Id {
//				return self.runtime.mainAtom
//			},
//			AtomMessages: nil,
//		},
//		AtomHandlers: nil,
//	}
//	return e
//}
//
//type mainElement struct {
//	self *CosmosSelf
//	atom *AtomCore
//}
//
//func newMainAtom(e *ElementLocal) *mainAtom {
//	a := allocAtomLocal()
//	e.current.Developer.(*mainElement).atom = a
//	initAtomLocal(a, e, MainAtomName, e.current, e.upgrades)
//	a.state = AtomosWaiting
//	e.atoms[MainAtomName] = a
//	_ = e.elementSpawningAtom(a, e.current, nil, nil)
//	a.element.cosmos.logInfo("Cosmos.Main: MainId is spawning")
//	return a.instance.(*mainAtom)
//}
//
//func (m *mainElement) AtomConstructor() Atom {
//	return &mainAtom{
//		self:     m.self,
//		AtomCore: m.atom,
//	}
//}
//
//// Main Atom
//
//type mainAtom struct {
//	self *CosmosSelf
//	*AtomCore
//}
//
//func (m *mainAtom) Connect(name, addr string) (CosmosNode, error) {
//	return m.self.Connect(name, addr)
//}
//
//func (m *mainAtom) Config() *Config {
//	return proto.Clone(m.self.config).(*Config)
//}
//
//func (m *mainAtom) CustomizeConfig(name string) (string, error) {
//	customize := m.Config().Customize
//	if customize == nil {
//		return "", errors.New("customize config has not defined")
//	}
//	value, has := customize[name]
//	if !has {
//		return "", errors.New("customize config key has not defined")
//	}
//	return value, nil
//}
//
//func (m *mainAtom) Halt(from ID, cancels map[uint64]CancelledTask) proto.Message {
//	m.self.logInfo("Cosmos.Main: MainId is halting")
//	return nil
//}
