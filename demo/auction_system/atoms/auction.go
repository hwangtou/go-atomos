package atoms

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/demo/auction_system/api"
	"log"
)

// Auction

type Auction struct {
	self       atomos.AtomSelf
	auction    api.Auction
	houseId    api.AtomAuctionHouseId
	watchers   map[string]api.AtomBidderId
	nextTaskId uint64
}

func (a *Auction) Spawn(self atomos.AtomSelf, data []byte) error {
	a.self = self
	// Recovery
	err := proto.Unmarshal(data, &a.auction)
	if err != nil {
		return err
	}
	// Get AuctionHouse
	a.houseId, err = api.GetAtomAuctionHouseId(a.self.Cosmos(), GlobalAuctionHouseName)
	if err != nil {
		return err
	}
	// Timer
	err, nextState, duration := a.auction.NextState()
	if err != nil {
		// The auction has end, if we want to look up its info, we should write back
		// to the Auction House
		return err
	}
	if duration > 0 {
		a.nextTaskId, err = a.self.Task().After(duration, a.nextState, &api.NextState{ Next: nextState })
	} else {
		a.nextTaskId, err = a.self.Task().Add(a.nextState, &api.NextState{ Next: nextState })
	}
	return err
}

func (a *Auction) Close(tasks map[uint64]proto.Message) []byte {
	d, err := proto.Marshal(&a.auction)
	if err != nil {
		log.Fatalln(err)
	}
	return d
}

func (a *Auction) Bid(from atomos.Id, in *api.BidReq) (*api.BidResp, error) {
	var err error
	var realPrice int32
	resp := &api.BidResp{
		Success: false,
		RealPrice: 0,
	}
	// Check state
	if a.auction.State != api.State_Bidding {
		return resp, errors.New("")
	}
	if a.auction.BidNow {
		return resp, errors.New("")
	}
	// Check bidder
	if err := in.Bidder.IsValid(); err != nil {
		return resp, err
	}
	// Bid
	if in.BidNow {
		realPrice, err = a.handleBuyNow(in.Bidder, in.BidPrice)
	} else {
		realPrice, err = a.handleBid(in.Bidder, in.BidPrice)
	}
	if err != nil {
		return resp, err
	}
	a.auction.AddBid(&api.Bid{
		Id:    in.Bidder.Id,
		Name:  in.Bidder.Name,
		Price: realPrice,
		BidAt: ptypes.TimestampNow(),
	})
	resp.Success = true
	resp.RealPrice = realPrice
	return resp, err
}

func (a *Auction) UpdateBidderWatcher(from atomos.Id, in *api.BidderWatcher) (*empty.Empty, error) {
	// For bidder atom only
	bidderName := from.Name()
	bidderId, err := api.GetAtomBidderId(from.Cosmos(), bidderName)
	if err != nil {
		return nil, err
	}
	if in.Watch {
		a.watchers[bidderName] = bidderId
		bidderId.UpdateAuction(a.self, &api.AuctionUpdate{
			Type:    0,
			Auction: nil,
		})
	} else {
		if _, has := a.watchers[from.Name()]; has {
			delete(a.watchers, from.Name())
		}
	}
	return nil, nil
}

func (a *Auction) nextState(state *api.NextState) {
	a.nextTaskId = 0
	a.auction.State = state.Next
	switch state.Next {
	case api.State_Bidding:
	case api.State_End:
	}
}

func (a *Auction) notifyUpdate(t api.UpdateType)  {
	if _, err := a.houseId.UpdateAuction(a.self, &api.AuctionUpdate{
		Type:    t,
		Auction: &a.auction,
	}); err != nil {
		a.self.Log().Error("notifyUpdate failed, err=%s", err)
	}
}

func (a *Auction) getNowPrice() int32 {
	if a.auction.LastBid == nil {
		return a.auction.ReservedPrice
	}
	return a.auction.LastBid.Price
}

func (a *Auction) handleBid(bidder *api.Bidder, price int32) (int32, error) {
	minPrice := a.getNowPrice() + a.auction.EachBinMin
	if minPrice > price {
		return 0, errors.New("auction.Bid: bidder balance not enough")
	}
	if minPrice >= a.auction.BuyNowPrice {
		return a.handleBuyNow(bidder, price)
	}
	return price, nil
}

func (a *Auction) handleBuyNow(bidder *api.Bidder, price int32) (int32, error) {
	if price < a.auction.BuyNowPrice {
		return 0, errors.New("")
	}
	if price > a.auction.BuyNowPrice {
		price = a.auction.BuyNowPrice
	}
	a.auction.BidNow = true
	return price, nil
}
