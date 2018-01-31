// Code generated by thriftrw v1.10.0. DO NOT EDIT.
// @generated

package enum_conflict

import (
	"go.uber.org/thriftrw/gen/testdata/enums"
	"go.uber.org/thriftrw/thriftreflect"
)

// ThriftModule represents the IDL file used to generate this package.
var ThriftModule = &thriftreflect.ThriftModule{
	Name:     "enum_conflict",
	Package:  "go.uber.org/thriftrw/gen/testdata/enum_conflict",
	FilePath: "enum_conflict.thrift",
	SHA1:     "75e0e6472e2f0c74412512d61531cf1a0da7429c",
	Includes: []*thriftreflect.ThriftModule{
		enums.ThriftModule,
	},
	Raw: rawIDL,
}

const rawIDL = "include \"./enums.thrift\"\n\nenum RecordType {\n    Name, Email\n}\n\nconst RecordType defaultRecordType = RecordType.Name\n\nconst enums.RecordType defaultOtherRecordType = enums.RecordType.NAME\n\nstruct Records {\n    1: optional RecordType recordType = defaultRecordType\n    2: optional enums.RecordType otherRecordType = defaultOtherRecordType\n}\n"
