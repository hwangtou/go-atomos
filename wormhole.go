package go_atomos

// 虫洞，从你的宇宙进入Cosmos宇宙的方法。
// Wormhole, the entrance of Cosmos, from your world.
type Wormhole interface {
	Name() string
	Boot() error
	BootFailed()
	Start(MainId)
	Stop()
	GetWorm(name string) Worm
}

type Worm interface {
	Name() string
	Receive([]byte) error
	Send([]byte) error
	AcceptNew()
	KickOld()
}
