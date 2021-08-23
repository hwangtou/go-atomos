module github.com/hwangtou/go-atomos/examples/chat

go 1.16

require (
	github.com/hwangtou/go-atomos v1.0.0
	github.com/hwangtou/go-atomos/examples/chat/api v0.0.0-00010101000000-000000000000
	github.com/hwangtou/go-atomos/examples/chat/element v0.0.0-00010101000000-000000000000
)

replace (
	github.com/hwangtou/go-atomos => ../../
	github.com/hwangtou/go-atomos/examples/chat/api => ./api
	github.com/hwangtou/go-atomos/examples/chat/element => ./element
)
