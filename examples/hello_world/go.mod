module github.com/hwangtou/go-atomos/examples

go 1.16

require (
	github.com/hwangtou/go-atomos v1.0.0
	github.com/hwangtou/go-atomos/examples/hello_world/api v0.0.0-00010101000000-000000000000
	github.com/hwangtou/go-atomos/examples/hello_world/element v0.0.0-00010101000000-000000000000
)

replace (
	github.com/hwangtou/go-atomos => ../../
	github.com/hwangtou/go-atomos/examples/hello_world/api => ./api
	github.com/hwangtou/go-atomos/examples/hello_world/element => ./element
)
