go build -o ../../../protoc-gen-go-atomos/protoc-gen-go-atomos ../../../protoc-gen-go-atomos/atomos.go ../../../protoc-gen-go-atomos/main.go

GP=$(printf %q "$GOPATH")
cp ../../../protoc-gen-go-atomos/protoc-gen-go "$GP"/bin/protoc-gen-go
cp ../../../protoc-gen-go-atomos/protoc-gen-go-atomos "$GP"/bin/protoc-gen-go-atomos

# shellcheck disable=SC2164
cd ../../../executable
go build main.go
