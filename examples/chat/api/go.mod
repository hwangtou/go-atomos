module github.com/hwangtou/go-atomos/examples/chat/api

go 1.16

require (
	github.com/hwangtou/go-atomos v1.0.0
)

replace (
	github.com/hwangtou/go-atomos => ../../../
	github.com/hwangtou/go-atomos/examples/chat/api => ./../api
)
