#!/bin/bash
# You need to install the proto compiler (protobuf-compiler OS package on
# Debian), and protoc-gen-go for go codegen.
# google.golang.org/protobuf/cmd/protoc-gen-go or protoc-gen-go OS package on
# Debian.

# Installation path of protoc-gen-go must be in your $PATH for the protoc to 
# find it. Usually protoc-gen-go is installed in $GOPATH/bin or $HOME/go/bin
protoc --go_out=. vm_test_info.proto
