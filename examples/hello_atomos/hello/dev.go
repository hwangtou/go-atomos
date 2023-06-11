package hello

import atomos "github.com/hwangtou/go-atomos"

// DevFeatures 决定一个Atomos的服务，它支持什么功能。
// Decide what features an Atomos service supports.
type DevFeatures interface {
	atomos.ElementDeveloper
}

type Dev struct {
}

func NewDev() DevFeatures {
	return &Dev{}
}

func (d *Dev) AtomConstructor(name string) atomos.Atomos {
	//TODO implement me
	panic("implement me")
}

func (d *Dev) ElementConstructor() atomos.Atomos {
	//TODO implement me
	panic("implement me")
}
