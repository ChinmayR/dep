// Code generated by thriftrw v1.8.0. DO NOT EDIT.
// @generated

package services

import (
	"fmt"
	"go.uber.org/thriftrw/wire"
	"strings"
)

// NonStandardServiceName_NonStandardFunctionName_Args represents the arguments for the non_standard_service_name.non_standard_function_name function.
//
// The arguments for non_standard_function_name are sent and received over the wire as this struct.
type NonStandardServiceName_NonStandardFunctionName_Args struct {
}

// ToWire translates a NonStandardServiceName_NonStandardFunctionName_Args struct into a Thrift-level intermediate
// representation. This intermediate representation may be serialized
// into bytes using a ThriftRW protocol implementation.
//
// An error is returned if the struct or any of its fields failed to
// validate.
//
//   x, err := v.ToWire()
//   if err != nil {
//     return err
//   }
//
//   if err := binaryProtocol.Encode(x, writer); err != nil {
//     return err
//   }
func (v *NonStandardServiceName_NonStandardFunctionName_Args) ToWire() (wire.Value, error) {
	var (
		fields [0]wire.Field
		i      int = 0
	)

	return wire.NewValueStruct(wire.Struct{Fields: fields[:i]}), nil
}

// FromWire deserializes a NonStandardServiceName_NonStandardFunctionName_Args struct from its Thrift-level
// representation. The Thrift-level representation may be obtained
// from a ThriftRW protocol implementation.
//
// An error is returned if we were unable to build a NonStandardServiceName_NonStandardFunctionName_Args struct
// from the provided intermediate representation.
//
//   x, err := binaryProtocol.Decode(reader, wire.TStruct)
//   if err != nil {
//     return nil, err
//   }
//
//   var v NonStandardServiceName_NonStandardFunctionName_Args
//   if err := v.FromWire(x); err != nil {
//     return nil, err
//   }
//   return &v, nil
func (v *NonStandardServiceName_NonStandardFunctionName_Args) FromWire(w wire.Value) error {

	for _, field := range w.GetStruct().Fields {
		switch field.ID {
		}
	}

	return nil
}

// String returns a readable string representation of a NonStandardServiceName_NonStandardFunctionName_Args
// struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Args) String() string {
	if v == nil {
		return "<nil>"
	}

	var fields [0]string
	i := 0

	return fmt.Sprintf("NonStandardServiceName_NonStandardFunctionName_Args{%v}", strings.Join(fields[:i], ", "))
}

// Equals returns true if all the fields of this NonStandardServiceName_NonStandardFunctionName_Args match the
// provided NonStandardServiceName_NonStandardFunctionName_Args.
//
// This function performs a deep comparison.
func (v *NonStandardServiceName_NonStandardFunctionName_Args) Equals(rhs *NonStandardServiceName_NonStandardFunctionName_Args) bool {

	return true
}

// MethodName returns the name of the Thrift function as specified in
// the IDL, for which this struct represent the arguments.
//
// This will always be "non_standard_function_name" for this struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Args) MethodName() string {
	return "non_standard_function_name"
}

// EnvelopeType returns the kind of value inside this struct.
//
// This will always be Call for this struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Args) EnvelopeType() wire.EnvelopeType {
	return wire.Call
}

// NonStandardServiceName_NonStandardFunctionName_Helper provides functions that aid in handling the
// parameters and return values of the non_standard_service_name.non_standard_function_name
// function.
var NonStandardServiceName_NonStandardFunctionName_Helper = struct {
	// Args accepts the parameters of non_standard_function_name in-order and returns
	// the arguments struct for the function.
	Args func() *NonStandardServiceName_NonStandardFunctionName_Args

	// IsException returns true if the given error can be thrown
	// by non_standard_function_name.
	//
	// An error can be thrown by non_standard_function_name only if the
	// corresponding exception type was mentioned in the 'throws'
	// section for it in the Thrift file.
	IsException func(error) bool

	// WrapResponse returns the result struct for non_standard_function_name
	// given the error returned by it. The provided error may
	// be nil if non_standard_function_name did not fail.
	//
	// This allows mapping errors returned by non_standard_function_name into a
	// serializable result struct. WrapResponse returns a
	// non-nil error if the provided error cannot be thrown by
	// non_standard_function_name
	//
	//   err := non_standard_function_name(args)
	//   result, err := NonStandardServiceName_NonStandardFunctionName_Helper.WrapResponse(err)
	//   if err != nil {
	//     return fmt.Errorf("unexpected error from non_standard_function_name: %v", err)
	//   }
	//   serialize(result)
	WrapResponse func(error) (*NonStandardServiceName_NonStandardFunctionName_Result, error)

	// UnwrapResponse takes the result struct for non_standard_function_name
	// and returns the erorr returned by it (if any).
	//
	// The error is non-nil only if non_standard_function_name threw an
	// exception.
	//
	//   result := deserialize(bytes)
	//   err := NonStandardServiceName_NonStandardFunctionName_Helper.UnwrapResponse(result)
	UnwrapResponse func(*NonStandardServiceName_NonStandardFunctionName_Result) error
}{}

func init() {
	NonStandardServiceName_NonStandardFunctionName_Helper.Args = func() *NonStandardServiceName_NonStandardFunctionName_Args {
		return &NonStandardServiceName_NonStandardFunctionName_Args{}
	}

	NonStandardServiceName_NonStandardFunctionName_Helper.IsException = func(err error) bool {
		switch err.(type) {
		default:
			return false
		}
	}

	NonStandardServiceName_NonStandardFunctionName_Helper.WrapResponse = func(err error) (*NonStandardServiceName_NonStandardFunctionName_Result, error) {
		if err == nil {
			return &NonStandardServiceName_NonStandardFunctionName_Result{}, nil
		}

		return nil, err
	}
	NonStandardServiceName_NonStandardFunctionName_Helper.UnwrapResponse = func(result *NonStandardServiceName_NonStandardFunctionName_Result) (err error) {
		return
	}

}

// NonStandardServiceName_NonStandardFunctionName_Result represents the result of a non_standard_service_name.non_standard_function_name function call.
//
// The result of a non_standard_function_name execution is sent and received over the wire as this struct.
type NonStandardServiceName_NonStandardFunctionName_Result struct {
}

// ToWire translates a NonStandardServiceName_NonStandardFunctionName_Result struct into a Thrift-level intermediate
// representation. This intermediate representation may be serialized
// into bytes using a ThriftRW protocol implementation.
//
// An error is returned if the struct or any of its fields failed to
// validate.
//
//   x, err := v.ToWire()
//   if err != nil {
//     return err
//   }
//
//   if err := binaryProtocol.Encode(x, writer); err != nil {
//     return err
//   }
func (v *NonStandardServiceName_NonStandardFunctionName_Result) ToWire() (wire.Value, error) {
	var (
		fields [0]wire.Field
		i      int = 0
	)

	return wire.NewValueStruct(wire.Struct{Fields: fields[:i]}), nil
}

// FromWire deserializes a NonStandardServiceName_NonStandardFunctionName_Result struct from its Thrift-level
// representation. The Thrift-level representation may be obtained
// from a ThriftRW protocol implementation.
//
// An error is returned if we were unable to build a NonStandardServiceName_NonStandardFunctionName_Result struct
// from the provided intermediate representation.
//
//   x, err := binaryProtocol.Decode(reader, wire.TStruct)
//   if err != nil {
//     return nil, err
//   }
//
//   var v NonStandardServiceName_NonStandardFunctionName_Result
//   if err := v.FromWire(x); err != nil {
//     return nil, err
//   }
//   return &v, nil
func (v *NonStandardServiceName_NonStandardFunctionName_Result) FromWire(w wire.Value) error {

	for _, field := range w.GetStruct().Fields {
		switch field.ID {
		}
	}

	return nil
}

// String returns a readable string representation of a NonStandardServiceName_NonStandardFunctionName_Result
// struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Result) String() string {
	if v == nil {
		return "<nil>"
	}

	var fields [0]string
	i := 0

	return fmt.Sprintf("NonStandardServiceName_NonStandardFunctionName_Result{%v}", strings.Join(fields[:i], ", "))
}

// Equals returns true if all the fields of this NonStandardServiceName_NonStandardFunctionName_Result match the
// provided NonStandardServiceName_NonStandardFunctionName_Result.
//
// This function performs a deep comparison.
func (v *NonStandardServiceName_NonStandardFunctionName_Result) Equals(rhs *NonStandardServiceName_NonStandardFunctionName_Result) bool {

	return true
}

// MethodName returns the name of the Thrift function as specified in
// the IDL, for which this struct represent the result.
//
// This will always be "non_standard_function_name" for this struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Result) MethodName() string {
	return "non_standard_function_name"
}

// EnvelopeType returns the kind of value inside this struct.
//
// This will always be Reply for this struct.
func (v *NonStandardServiceName_NonStandardFunctionName_Result) EnvelopeType() wire.EnvelopeType {
	return wire.Reply
}
