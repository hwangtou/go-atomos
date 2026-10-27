package atomostest

import (
	"context"
	"sync"
	"time"

	"github.com/hwangtou/go-atomos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FaultAction identifies a transport fault. DropResponse invokes the real
// handler before returning an error; DropRequest does not invoke it.
type FaultAction int

const (
	FaultDropRequest FaultAction = iota + 1
	FaultDropResponse
	FaultDelay
	FaultHold
	FaultDuplicateRequest
	FaultUnavailable
	FaultBlackhole
)

// FaultRule matches selected RPCs. Zero numeric fields match any value. Hold
// and delay rules can be released or advanced independently of real time.
type FaultRule struct {
	Source      string
	Destination string
	Method      string
	OperationID string
	MessageID   string
	Attempt     int
	Occurrence  int
	Action      FaultAction
	Duration    time.Duration
	Once        bool
}

// FaultEvent records one injected fault in controller order.
type FaultEvent struct {
	Seq         uint64
	Time        time.Time
	Source      string
	Destination string
	Method      string
	OperationID string
	MessageID   string
	Attempt     int
	Occurrence  int
	Action      FaultAction
}

// FaultController injects deterministic client-side gRPC faults.
type FaultController struct {
	mutex       sync.Mutex
	source      string
	destination string
	clock       atomos.Clock
	rules       map[int]*faultRuleState
	events      []FaultEvent
	attempts    map[string]int
	holds       map[uint64]chan struct{}
	nextRuleID  int
	matches     int
}

type faultRuleState struct {
	rule FaultRule
}

// NewFaultController creates a controller for one source/destination pair.
// A nil clock uses real time for delays and event timestamps.
func NewFaultController(source, destination string, clock atomos.Clock) *FaultController {
	return &FaultController{
		source: source, destination: destination, clock: clock,
		rules: map[int]*faultRuleState{}, attempts: map[string]int{}, holds: map[uint64]chan struct{}{},
	}
}

// AddRule registers a fault and returns its rule ID.
func (c *FaultController) AddRule(rule FaultRule) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nextRuleID++
	id := c.nextRuleID
	c.rules[id] = &faultRuleState{rule: rule}
	return id
}

// RemoveRule disables one fault rule.
func (c *FaultController) RemoveRule(id int) {
	c.mutex.Lock()
	delete(c.rules, id)
	c.mutex.Unlock()
}

// Heal removes all rules and releases held calls.
func (c *FaultController) Heal() {
	c.mutex.Lock()
	c.rules = map[int]*faultRuleState{}
	holds := c.holds
	c.holds = map[uint64]chan struct{}{}
	c.mutex.Unlock()
	for _, hold := range holds {
		close(hold)
	}
}

// Release releases a held event by sequence number.
func (c *FaultController) Release(seq uint64) bool {
	c.mutex.Lock()
	hold := c.holds[seq]
	delete(c.holds, seq)
	c.mutex.Unlock()
	if hold == nil {
		return false
	}
	close(hold)
	return true
}

// Events returns a snapshot of injected faults.
func (c *FaultController) Events() []FaultEvent {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]FaultEvent(nil), c.events...)
}

// UnaryClientInterceptor returns an interceptor that preserves the real
// service invocation for response-loss and duplicate-request faults.
func (c *FaultController) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		operationID, messageID := faultRequestIDs(req)
		key := method + "\x00" + operationID + "\x00" + messageID
		c.mutex.Lock()
		c.attempts[key]++
		attempt := c.attempts[key]
		c.mutex.Unlock()

		for {
			rule, occurrence := c.match(method, operationID, messageID, attempt)
			if rule.Action == 0 {
				return invoker(ctx, method, req, reply, conn, opts...)
			}
			event := c.record(rule, method, operationID, messageID, attempt, occurrence)

			switch rule.Action {
			case FaultDropRequest, FaultUnavailable:
				return status.Error(codes.Unavailable, "injected request failure")
			case FaultDropResponse:
				err := invoker(ctx, method, req, reply, conn, opts...)
				if err != nil {
					return err
				}
				return status.Error(codes.Unavailable, "injected response loss")
			case FaultDuplicateRequest:
				if err := invoker(ctx, method, req, reply, conn, opts...); err != nil {
					return err
				}
				return invoker(ctx, method, req, reply, conn, opts...)
			case FaultDelay:
				if err := c.wait(ctx, rule.Duration); err != nil {
					return err
				}
				return invoker(ctx, method, req, reply, conn, opts...)
			case FaultHold:
				if err := c.hold(ctx, event.Seq); err != nil {
					return err
				}
				return invoker(ctx, method, req, reply, conn, opts...)
			case FaultBlackhole:
				<-ctx.Done()
				return ctx.Err()
			default:
				return status.Error(codes.Internal, "unknown injected fault")
			}
		}
	}
}

func (c *FaultController) match(method, operationID, messageID string, attempt int) (FaultRule, int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for id, state := range c.rules {
		rule := state.rule
		if rule.Source != "" && rule.Source != c.source ||
			rule.Destination != "" && rule.Destination != c.destination ||
			rule.Method != "" && rule.Method != method ||
			rule.OperationID != "" && rule.OperationID != operationID ||
			rule.MessageID != "" && rule.MessageID != messageID ||
			rule.Attempt != 0 && rule.Attempt != attempt ||
			rule.Occurrence != 0 && rule.Occurrence != c.matches+1 {
			continue
		}
		c.matches++
		occurrence := c.matches
		if rule.Once {
			delete(c.rules, id)
		}
		return rule, occurrence
	}
	return FaultRule{}, 0
}

func (c *FaultController) record(rule FaultRule, method, operationID, messageID string, attempt, occurrence int) FaultEvent {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := time.Now()
	if c.clock != nil {
		now = c.clock.Now()
	}
	event := FaultEvent{
		Seq: uint64(len(c.events) + 1), Time: now, Source: c.source, Destination: c.destination,
		Method: method, OperationID: operationID, MessageID: messageID,
		Attempt: attempt, Occurrence: occurrence, Action: rule.Action,
	}
	c.events = append(c.events, event)
	return event
}

func (c *FaultController) wait(ctx context.Context, d time.Duration) error {
	var ch <-chan time.Time
	if c.clock == nil {
		ch = time.After(d)
	} else {
		ch = c.clock.After(d)
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *FaultController) hold(ctx context.Context, seq uint64) error {
	c.mutex.Lock()
	hold := make(chan struct{})
	c.holds[seq] = hold
	c.mutex.Unlock()
	select {
	case <-hold:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func faultRequestIDs(req any) (operationID, messageID string) {
	if req, ok := req.(interface{ GetOperationId() string }); ok {
		operationID = req.GetOperationId()
	}
	if req, ok := req.(interface{ GetMessageId() string }); ok {
		messageID = req.GetMessageId()
	}
	return operationID, messageID
}
