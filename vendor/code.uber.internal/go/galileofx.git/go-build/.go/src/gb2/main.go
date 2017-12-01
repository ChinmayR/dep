// This is a dummy Go file that is used to list the binary dependencies we have.
// We blank import all packages that we want to depend on, and use glide to manage
// the vendor directory with the code for the binaries and their dependencies.
package main

import (
	_ "code.uber.internal/devexp/homebrew_bottler"
	_ "code.uber.internal/infra/glide-exec"
	_ "code.uber.internal/infra/prototools.git/cmd/go-build-protoc-helper"
	_ "github.com/AlekSi/gocov-xml"
	_ "github.com/Masterminds/glide"
	_ "github.com/Masterminds/vcs"
	_ "github.com/axw/gocov/gocov"
	_ "github.com/gogo/protobuf/proto"
	_ "github.com/gogo/protobuf/protoc-gen-gogofaster"
	_ "github.com/gogo/protobuf/protoc-gen-gogoslick"
	_ "github.com/golang/lint/golint"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/proto"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/jstemmer/go-junit-report"
	_ "github.com/jteeuwen/go-bindata"
	_ "github.com/kisielk/errcheck"
	_ "github.com/matm/gocov-html"
	_ "github.com/pquerna/ffjson"
	_ "github.com/tools/godep"
	_ "github.com/uber/tchannel-go/thrift/thrift-gen"
	_ "github.com/vektra/mockery/cmd/mockery"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "honnef.co/go/tools/cmd/unused"
)
