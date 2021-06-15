module github.com/hwangtou/go-atomos/examples/hello_world/api

go 1.16

require (
	github.com/golang/protobuf v1.5.2
	github.com/hwangtou/go-atomos v1.0.0
)

replace (
	github.com/hwangtou/go-atomos => ../../../
	github.com/hwangtou/go-atomos/examples/hello_world/api => ./../api/
)
