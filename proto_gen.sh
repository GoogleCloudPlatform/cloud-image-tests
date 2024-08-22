#!/bin/bash

# Installation path of protoc-gen-go must be in your $PATH for the protoc to 
# find it. Usually protoc-gen-go is installed in $GOPATH/bin or $HOME/go/bin
sudo apt install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
protoc --go_out=. vm_test_info.proto
