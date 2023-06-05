#!/bin/bash

# Get the absolute path of the protocol buffer file
PROTOCOL_PATH=$(readlink -f "$0")
#echo "The protocol is located at $PROTOCOL_PATH"

# Get the directory that the protocol buffer file is in
PROTOCOL_DIR=$(dirname "$PROTOCOL_PATH")
echo "The protocol's directory is $PROTOCOL_DIR"

# Get the atomos directory that the atomos.proto is in
ATOMOS_DIR=$(dirname "$(dirname "$PROTOCOL_DIR")")
echo "The atomos directory is $ATOMOS_DIR"

echo "Generating protocol buffer files..."

gen_protocol() {
  protoc -I="$ATOMOS_DIR" --go_out=. --go-atomos_out=. $PROTOCOL_DIR/$1
  result=$?
  echo "file: $1 -> protoc result: $result"
}

gen_protocol "api/hello.proto"
