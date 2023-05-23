package go_atomos

import (
	etcdCli "go.etcd.io/etcd/client/v3"
	"time"
)

func connectEtcd(endpoints []string) (*etcdCli.Client, *Error) {
	client, er := etcdCli.New(etcdCli.Config{
		Endpoints:   endpoints,
		DialTimeout: 1 * time.Second,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosGlobalEtcdConnectFailed, "CosmosGlobal: Etcd connect failed. err=(%v)", er)
	}
	return client, nil
}
