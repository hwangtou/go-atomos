package go_atomos

import (
	"testing"
)

func TestCosmosMain(t *testing.T) {
	initTestFakeCosmosProcess(t)
	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t)); err != nil {
		t.Errorf("CosmosMain: Start failed. err=(%v)", err)
		return
	}
	if err := SharedCosmosProcess().Stop(); err != nil {
		t.Errorf("CosmosMain: Stop failed. err=(%v)", err)
		return
	}
}
