go build -o ../../protoc-gen-go-atomos/protoc-gen-go-atomos ../../protoc-gen-go-atomos/atomos.go ../../protoc-gen-go-atomos/main.go

GP=$(printf %q "$GOPATH")
echo $GP
cp ../../protoc-gen-go-atomos/protoc-gen-go "$GP"/bin/protoc-gen-go
cp ../../protoc-gen-go-atomos/protoc-gen-go-atomos "$GP"/bin/protoc-gen-go-atomos

# shellcheck disable=SC2164
protoc --go_out=../.. --go-atomos_out=../.. api/hello.proto
go build main.go runnable.go
