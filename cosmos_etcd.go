package go_atomos

//import (
//	etcdClient "go.etcd.io/etcd/client/v3"
//	"time"
//)
//
//func connectEtcd(endpoints []string) (*etcdClient.Client, *Error) {
//	client, er := etcdClient.New(etcdClient.Config{
//		Endpoints:   endpoints,
//		DialTimeout: 1 * time.Second,
//	})
//	if er != nil {
//		return nil, NewErrorf(ErrCosmosGlobalEtcdConnectFailed, "CosmosGlobal: Etcd connect failed. err=(%v)", er)
//	}
//	return client, nil
//}
