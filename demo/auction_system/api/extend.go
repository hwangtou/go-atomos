package api

import (
	"errors"
	"github.com/golang/protobuf/ptypes"
	"time"
)

// Auction

func (x *Auction) CreateTime() (time.Time, error) {
	return ptypes.Timestamp(x.CreatedAt)
}

func (x *Auction) BiddingTime() (time.Time, error) {
	return ptypes.Timestamp(x.BiddingAt)
}

func (x *Auction) EndTime() (time.Time, error) {
	return ptypes.Timestamp(x.EndAt)
}

func (x *Auction) IsValid() error {
	create, err := x.CreateTime()
	if err != nil {
		return err
	}
	bidding, err := x.BiddingTime()
	if err != nil {
		return err
	}
	end, err := x.EndTime()
	if err != nil {
		return err
	}
	if !create.Before(bidding) {
		return errors.New("")
	}
	if !bidding.Before(end) {
		return errors.New("")
	}
	return nil
}

func (x *Auction) NextState() (error, State, time.Duration) {
	now := time.Now()
	switch x.State {
	case State_New:
		t, _ := x.CreateTime()
		r := now.Sub(t)
		return nil, State_Bidding, r
	case State_Bidding:
		t, _ := x.BiddingTime()
		r := now.Sub(t)
		return nil, State_End, r
	}
	return ErrAuctionEnded, State_End, 0
}

func (x *Auction) AddBid(bid *Bid) {
	oldBid := x.LastBid
	if oldBid != nil {
		x.BidLog = append(x.BidLog, oldBid)
	}
	x.LastBid = bid
}

func (x *Auction) UpdateType() UpdateType {
	switch x.State {
	case State_New:
		return UpdateType_NewBid
	case State_Bidding:
		return UpdateType_BidNew
	case State_End:
		return UpdateType_BidEnd
	}
	return UpdateType_BidEnd
}

// Bidder

func (x *Bidder) IsValid() error {
	if x.Id == "" {
		return errors.New("")
	}
	if x.Name == "" {
		return errors.New("")
	}
	return nil
}

func (x *Bidder) HasBalance(amount int32) bool {
	return x.Balance >= amount
}
