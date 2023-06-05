go build -o protoc-gen-go-atomos/protoc-gen-go-atomos protoc-gen-go-atomos/atomos.go protoc-gen-go-atomos/main.go

GOPATH="/Users/hwangtou/Developer/go"

cp protoc-gen-go-atomos/protoc-gen-go-atomos "$GOPATH"/bin/protoc-gen-go-atomos
protoc --go_out=. --go-atomos_out=. examples/hello_world/api/hello.proto
