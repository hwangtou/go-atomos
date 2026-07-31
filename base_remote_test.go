package atomos

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestBaseRemote_LifeCycle

func TestBaseRemote_LifeCycle(t *testing.T) {

}

// TestBaseRemote_PinnedConnConcurrentAccess hammers validPinnedConn from
// multiple goroutines while the pinned connection is closed, forcing the
// Shutdown-detection clear path to race with concurrent reads. Before the
// pinnedConnBox guard this tripped -race (unsynchronized pinnedConn
// read/write); it must stay clean.
func TestBaseRemote_PinnedConnConcurrentAccess(t *testing.T) {
	// NewClient never connects by itself; we only exercise GetState/Close.
	conn, err := grpc.NewClient("passthrough:///127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	b := newBaseRemoteWithPinnedConn(nil, nil, conn)

	const readers = 8
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = b.validPinnedConn()
				_ = b.getPinnedConn()
			}
		}()
	}

	// Let readers hammer the live pin, then close the conn so the clear path
	// runs concurrently with the reads.
	time.Sleep(50 * time.Millisecond)
	if er := conn.Close(); er != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("conn.Close failed: %v", er)
	}

	// Any reader may be the one to detect Shutdown and clear the pin.
	deadline := time.Now().Add(5 * time.Second)
	for b.getPinnedConn() != nil {
		if time.Now().After(deadline) {
			close(stop)
			wg.Wait()
			t.Fatal("pinned conn was not cleared after Close")
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if c := b.validPinnedConn(); c != nil {
		t.Fatalf("validPinnedConn should stay nil after clear, got %v", c)
	}
}

// internal

func newBaseRemoteForTest(t *testing.T, cosmos *CosmosRemote, info *IDInfo) *BaseRemote {
	return &BaseRemote{
		cosmos: cosmos,
		info:   info,
		pinned: &pinnedConnBox{},
	}
}
