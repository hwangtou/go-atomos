package atomos

import "testing"

func TestQueueEtcdInfoCoalescesToLatestCompleteState(t *testing.T) {
	process := &CosmosProcess{}
	process.cluster.etcdInfoCh = make(chan etcdInfoUpdate, 1)
	process.queueEtcdInfo(`{"State":0,"Node":"node","StartupId":7}`)
	process.queueEtcdInfo(`{"State":1,"Node":"node","StartupId":7}`)

	updated := <-process.cluster.etcdInfoCh
	if updated.seq != 2 {
		t.Fatalf("latest update seq = %d, want 2", updated.seq)
	}
	if updated.info != `{"State":1,"Node":"node","StartupId":7}` {
		t.Fatalf("latest update = %s", updated.info)
	}
	process.queueEtcdInfo(`{"State":1,"Node":"node","StartupId":8}`)
	updated = <-process.cluster.etcdInfoCh
	if updated.seq != 3 || updated.info != `{"State":1,"Node":"node","StartupId":8}` {
		t.Fatalf("subsequent update = (%d, %s), want monotonic seq and complete state", updated.seq, updated.info)
	}
}
