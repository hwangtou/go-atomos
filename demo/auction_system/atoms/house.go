package atoms

import (
	"fmt"
	"github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/demo/auction_system/api"
)

// AuctionHouse

type AuctionHouse struct {
	self atomos.AtomSelf
	auctions map[string]api.AtomAuctionId
	watcher  map[string]api.AtomBidderId
}

var GlobalAuctionHouseName = "AuctionHouse"

func (h *AuctionHouse) Spawn(self atomos.AtomSelf, data []byte) error {
	h.self = self
	return nil
}

func (h *AuctionHouse) Save() []byte {
	return nil
}

func (h *AuctionHouse) NewAuction(req *api.NewAuctionReq) (*api.NewAuctionResp, error) {
	resp := &api.NewAuctionResp{}
	a := req.Auction
	if err := a.IsValid(); err != nil {
		return resp, err
	}
	name := fmt.Sprintf("Auction:%s", a.Name)
	a.Name = name
	auction := &Auction{}
	auction.auction = *a
	ref, err := api.SpawnAtomAuction(h.self.SelfCosmos(), name, auction)
	if err != nil {
		return resp, err
	}
	h.auctions[name] = ref
	return resp, nil
}

func (h *AuctionHouse) WatchAuctions(req *api.WatchAuctionsReq) (*api.WatchAuctionsResp, error) {
	resp := &api.WatchAuctionsResp{}
	b, err := api.GetAtomBidderId(h.self.Cosmos(), req.Bid.Id)
	if err != nil {
		return resp, err
	}
	h.watcher[req.Bid.Id] = b
	return resp, nil
}
