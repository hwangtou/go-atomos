package atomos

import (
	"testing"
)

func TestCosmosMain(t *testing.T) {
	initTestFakeCosmosProcess(t)
	if _, err := SharedCosmosProcess().start(newTestFakeRunnable(t, sharedCosmosProcess, false)); err != nil {
		t.Errorf("CosmosLocal: Start failed. err=(%v)", err)
		return
	}
	if err := SharedCosmosProcess().Stop(); err != nil {
		t.Errorf("CosmosLocal: Stop failed. err=(%v)", err)
		return
	}
}
