go build -o protoc-gen-go-atomos/protoc-gen-go-atomos protoc-gen-go-atomos/atomos.go protoc-gen-go-atomos/main.go

cp protoc-gen-go-atomos/protoc-gen-go "$GOPATH"/bin/protoc-gen-go
cp protoc-gen-go-atomos/protoc-gen-go-atomos "$GOPATH"/bin/protoc-gen-go-atomos
protoc --go_out=. --go-atomos_out=. examples/hello_world/api/helloworld.proto
protoc --go_out=. --go-atomos_out=. examples/chat/api/chat.proto
