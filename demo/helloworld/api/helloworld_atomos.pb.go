// Code generated by protoc-gen-go-atomos. DO NOT EDIT.

package api

import (
	proto "github.com/golang/protobuf/proto"
	go_atomos "github.com/hwangtou/go-atomos"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the atomos package it is being compiled against.

//
// PUBLIC
//

// GreeterId is the interface of Greeter atomos.
//
type GreeterId interface {
	go_atomos.Id
	SayHello(from go_atomos.Id, in *HelloRequest) (*HelloReply, error)
}

func GetGreeterId(c go_atomos.CosmosNode, name string) (GreeterId, error) {
	ca, err := c.GetAtomId(&GreeterDesc, name)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(GreeterId); ok {
		return c, nil
	} else {
		return nil, go_atomos.ErrCustomizeAtomType
	}
}

// GreeterAtom is the atomos implements of Greeter atomos.
//
type GreeterAtom interface {
	go_atomos.Atom
	SayHello(from go_atomos.Id, in *HelloRequest) (*HelloReply, error)
}

func SpawnGreeter(c *go_atomos.Cosmos, name string, atom GreeterAtom) (GreeterId, error) {
	ca, err := c.SpawnAtom(&GreeterDesc, name, atom)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(GreeterId); ok {
		return c, nil
	}
	return nil, go_atomos.ErrCustomizeAtomType
}

//
// INTERNAL
//

type greeterId struct {
	world go_atomos.CosmosNode
	aType string
	aName string
}

func (c *greeterId) Cosmos() go_atomos.CosmosNode {
	return c.world
}

func (c *greeterId) Type() string {
	return c.aType
}

func (c *greeterId) Name() string {
	return c.aName
}

func (c *greeterId) Kill(from go_atomos.Id) error {
	return c.world.CloseAtom(from, GreeterDesc.Name, c.aName)
}

func (c *greeterId) SayHello(from go_atomos.Id, in *HelloRequest) (*HelloReply, error) {
	r, err := c.world.CallAtom(from, GreeterDesc.Name, c.aName, "SayHello", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*HelloReply)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

var GreeterDesc go_atomos.AtomTypeDesc = go_atomos.AtomTypeDesc{
	Name: "api.Greeter",
	NewId: func(c go_atomos.CosmosNode, name string) go_atomos.Id {
		return &greeterId{c, "api.Greeter", name}
	},
	Calls: []go_atomos.CallDesc{
		{
			Name: "SayHello",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*HelloRequest)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(GreeterAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.SayHello(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &HelloRequest{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &HelloReply{}
				return r, proto.Unmarshal(buf, r)
			},
		},
	},
}
