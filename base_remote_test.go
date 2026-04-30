package atomos

import "testing"

// TestBaseRemote_LifeCycle

func TestBaseRemote_LifeCycle(t *testing.T) {

}

// internal

func newBaseRemoteForTest(t *testing.T, cosmos *CosmosRemote, info *IDInfo) *BaseRemote {
	return &BaseRemote{
		cosmos: cosmos,
		info:   info,
	}
}
