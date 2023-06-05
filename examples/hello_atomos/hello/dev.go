package hello

import (
	atomos "github.com/hwangtou/go-atomos"
	"hello_atomos/hello/views"
)

// DevFeatures 决定一个Atomos的服务，它支持什么功能。
// Decide what features an Atomos service supports.
type DevFeatures interface {
	atomos.ElementDeveloper
}

type dev struct{}

func NewDev() DevFeatures {
	return &dev{}
}

func (d *dev) AtomConstructor(name string) atomos.Atomos {
	return views.NewAtom(name)
}

func (d *dev) ElementConstructor() atomos.Atomos {
	return views.NewElement()
}
