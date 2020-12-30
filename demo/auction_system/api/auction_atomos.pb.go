// Code generated by protoc-gen-go-atomos. DO NOT EDIT.

package api

import (
	proto "github.com/golang/protobuf/proto"
	empty "github.com/golang/protobuf/ptypes/empty"
	go_atomos "github.com/hwangtou/go-atomos"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the atomos package it is being compiled against.

//
// PUBLIC
//

// AtomAuctionHouseId is the interface of AtomAuctionHouse atomos.
//
type AtomAuctionHouseId interface {
	go_atomos.Id
	NewAuction(from go_atomos.Id, in *NewAuctionReq) (*NewAuctionResp, error)
	WatchAuctions(from go_atomos.Id, in *WatchAuctionsReq) (*WatchAuctionsResp, error)
	UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*empty.Empty, error)
}

func GetAtomAuctionHouseId(c go_atomos.CosmosNode, name string) (AtomAuctionHouseId, error) {
	ca, err := c.GetAtomId(&AtomAuctionHouseDesc, name)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomAuctionHouseId); ok {
		return c, nil
	} else {
		return nil, go_atomos.ErrCustomizeAtomType
	}
}

// AtomAuctionHouseAtom is the atomos implements of AtomAuctionHouse atomos.
//
type AtomAuctionHouseAtom interface {
	go_atomos.Atom
	NewAuction(from go_atomos.Id, in *NewAuctionReq) (*NewAuctionResp, error)
	WatchAuctions(from go_atomos.Id, in *WatchAuctionsReq) (*WatchAuctionsResp, error)
	UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*empty.Empty, error)
}

func SpawnAtomAuctionHouse(c *go_atomos.Cosmos, name string, atom AtomAuctionHouseAtom) (AtomAuctionHouseId, error) {
	ca, err := c.SpawnAtom(&AtomAuctionHouseDesc, name, atom)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomAuctionHouseId); ok {
		return c, nil
	}
	return nil, go_atomos.ErrCustomizeAtomType
}

// AtomAuctionId is the interface of AtomAuction atomos.
//
type AtomAuctionId interface {
	go_atomos.Id
	UpdateBidderWatcher(from go_atomos.Id, in *BidderWatcher) (*empty.Empty, error)
	Bid(from go_atomos.Id, in *BidReq) (*BidResp, error)
}

func GetAtomAuctionId(c go_atomos.CosmosNode, name string) (AtomAuctionId, error) {
	ca, err := c.GetAtomId(&AtomAuctionDesc, name)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomAuctionId); ok {
		return c, nil
	} else {
		return nil, go_atomos.ErrCustomizeAtomType
	}
}

// AtomAuctionAtom is the atomos implements of AtomAuction atomos.
//
type AtomAuctionAtom interface {
	go_atomos.Atom
	UpdateBidderWatcher(from go_atomos.Id, in *BidderWatcher) (*empty.Empty, error)
	Bid(from go_atomos.Id, in *BidReq) (*BidResp, error)
}

func SpawnAtomAuction(c *go_atomos.Cosmos, name string, atom AtomAuctionAtom) (AtomAuctionId, error) {
	ca, err := c.SpawnAtom(&AtomAuctionDesc, name, atom)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomAuctionId); ok {
		return c, nil
	}
	return nil, go_atomos.ErrCustomizeAtomType
}

// AtomBidderId is the interface of AtomBidder atomos.
//
type AtomBidderId interface {
	go_atomos.Id
	UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*Resp, error)
}

func GetAtomBidderId(c go_atomos.CosmosNode, name string) (AtomBidderId, error) {
	ca, err := c.GetAtomId(&AtomBidderDesc, name)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomBidderId); ok {
		return c, nil
	} else {
		return nil, go_atomos.ErrCustomizeAtomType
	}
}

// AtomBidderAtom is the atomos implements of AtomBidder atomos.
//
type AtomBidderAtom interface {
	go_atomos.Atom
	UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*Resp, error)
}

func SpawnAtomBidder(c *go_atomos.Cosmos, name string, atom AtomBidderAtom) (AtomBidderId, error) {
	ca, err := c.SpawnAtom(&AtomBidderDesc, name, atom)
	if err != nil {
		return nil, err
	}
	if c, ok := ca.(AtomBidderId); ok {
		return c, nil
	}
	return nil, go_atomos.ErrCustomizeAtomType
}

//
// INTERNAL
//

type atomAuctionHouseId struct {
	world go_atomos.CosmosNode
	aType string
	aName string
}

func (c *atomAuctionHouseId) Cosmos() go_atomos.CosmosNode {
	return c.world
}

func (c *atomAuctionHouseId) Type() string {
	return c.aType
}

func (c *atomAuctionHouseId) Name() string {
	return c.aName
}

func (c *atomAuctionHouseId) Kill(from go_atomos.Id) error {
	return c.world.CloseAtom(from, AtomAuctionHouseDesc.Name, c.aName)
}

func (c *atomAuctionHouseId) NewAuction(from go_atomos.Id, in *NewAuctionReq) (*NewAuctionResp, error) {
	r, err := c.world.CallAtom(from, AtomAuctionHouseDesc.Name, c.aName, "NewAuction", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*NewAuctionResp)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

func (c *atomAuctionHouseId) WatchAuctions(from go_atomos.Id, in *WatchAuctionsReq) (*WatchAuctionsResp, error) {
	r, err := c.world.CallAtom(from, AtomAuctionHouseDesc.Name, c.aName, "WatchAuctions", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*WatchAuctionsResp)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

func (c *atomAuctionHouseId) UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*empty.Empty, error) {
	r, err := c.world.CallAtom(from, AtomAuctionHouseDesc.Name, c.aName, "UpdateAuction", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*empty.Empty)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

var AtomAuctionHouseDesc go_atomos.AtomTypeDesc = go_atomos.AtomTypeDesc{
	Name: "api.AtomAuctionHouse",
	NewId: func(c go_atomos.CosmosNode, name string) go_atomos.Id {
		return &atomAuctionHouseId{c, "api.AtomAuctionHouse", name}
	},
	Calls: []go_atomos.CallDesc{
		{
			Name: "NewAuction",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*NewAuctionReq)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomAuctionHouseAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.NewAuction(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &NewAuctionReq{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &NewAuctionResp{}
				return r, proto.Unmarshal(buf, r)
			},
		},
		{
			Name: "WatchAuctions",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*WatchAuctionsReq)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomAuctionHouseAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.WatchAuctions(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &WatchAuctionsReq{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &WatchAuctionsResp{}
				return r, proto.Unmarshal(buf, r)
			},
		},
		{
			Name: "UpdateAuction",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*AuctionUpdate)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomAuctionHouseAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.UpdateAuction(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &AuctionUpdate{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &empty.Empty{}
				return r, proto.Unmarshal(buf, r)
			},
		},
	},
}

type atomAuctionId struct {
	world go_atomos.CosmosNode
	aType string
	aName string
}

func (c *atomAuctionId) Cosmos() go_atomos.CosmosNode {
	return c.world
}

func (c *atomAuctionId) Type() string {
	return c.aType
}

func (c *atomAuctionId) Name() string {
	return c.aName
}

func (c *atomAuctionId) Kill(from go_atomos.Id) error {
	return c.world.CloseAtom(from, AtomAuctionDesc.Name, c.aName)
}

func (c *atomAuctionId) UpdateBidderWatcher(from go_atomos.Id, in *BidderWatcher) (*empty.Empty, error) {
	r, err := c.world.CallAtom(from, AtomAuctionDesc.Name, c.aName, "UpdateBidderWatcher", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*empty.Empty)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

func (c *atomAuctionId) Bid(from go_atomos.Id, in *BidReq) (*BidResp, error) {
	r, err := c.world.CallAtom(from, AtomAuctionDesc.Name, c.aName, "Bid", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*BidResp)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

var AtomAuctionDesc go_atomos.AtomTypeDesc = go_atomos.AtomTypeDesc{
	Name: "api.AtomAuction",
	NewId: func(c go_atomos.CosmosNode, name string) go_atomos.Id {
		return &atomAuctionId{c, "api.AtomAuction", name}
	},
	Calls: []go_atomos.CallDesc{
		{
			Name: "UpdateBidderWatcher",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*BidderWatcher)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomAuctionAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.UpdateBidderWatcher(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &BidderWatcher{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &empty.Empty{}
				return r, proto.Unmarshal(buf, r)
			},
		},
		{
			Name: "Bid",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*BidReq)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomAuctionAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.Bid(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &BidReq{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &BidResp{}
				return r, proto.Unmarshal(buf, r)
			},
		},
	},
}

type atomBidderId struct {
	world go_atomos.CosmosNode
	aType string
	aName string
}

func (c *atomBidderId) Cosmos() go_atomos.CosmosNode {
	return c.world
}

func (c *atomBidderId) Type() string {
	return c.aType
}

func (c *atomBidderId) Name() string {
	return c.aName
}

func (c *atomBidderId) Kill(from go_atomos.Id) error {
	return c.world.CloseAtom(from, AtomBidderDesc.Name, c.aName)
}

func (c *atomBidderId) UpdateAuction(from go_atomos.Id, in *AuctionUpdate) (*Resp, error) {
	r, err := c.world.CallAtom(from, AtomBidderDesc.Name, c.aName, "UpdateAuction", in)
	if err != nil {
		return nil, err
	}
	reply, ok := r.(*Resp)
	if !ok {
		return nil, go_atomos.ErrAtomCallNotExists
	}
	return reply, nil
}

var AtomBidderDesc go_atomos.AtomTypeDesc = go_atomos.AtomTypeDesc{
	Name: "api.AtomBidder",
	NewId: func(c go_atomos.CosmosNode, name string) go_atomos.Id {
		return &atomBidderId{c, "api.AtomBidder", name}
	},
	Calls: []go_atomos.CallDesc{
		{
			Name: "UpdateAuction",
			Func: func(from go_atomos.Id, to go_atomos.Atom, in proto.Message) (proto.Message, error) {
				req, ok := in.(*AuctionUpdate)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				a, ok := to.(AtomBidderAtom)
				if !ok {
					return nil, go_atomos.ErrAtomTypeNotExists
				}
				return a.UpdateAuction(from, req)
			},
			ArgDec: func(buf []byte) (proto.Message, error) {
				r := &AuctionUpdate{}
				return r, proto.Unmarshal(buf, r)
			},
			ReplyDec: func(buf []byte) (proto.Message, error) {
				r := &Resp{}
				return r, proto.Unmarshal(buf, r)
			},
		},
	},
}