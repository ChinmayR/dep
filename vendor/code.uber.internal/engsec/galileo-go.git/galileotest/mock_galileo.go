// Code generated by MockGen. DO NOT EDIT.
// Source: code.uber.internal/engsec/galileo-go.git (interfaces: Galileo)

package galileotest

import (
	galileo_go_git "code.uber.internal/engsec/galileo-go.git"
	context "context"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockGalileo is a mock of Galileo interface
type MockGalileo struct {
	ctrl     *gomock.Controller
	recorder *MockGalileoMockRecorder
}

// MockGalileoMockRecorder is the mock recorder for MockGalileo
type MockGalileoMockRecorder struct {
	mock *MockGalileo
}

// NewMockGalileo creates a new mock instance
func NewMockGalileo(ctrl *gomock.Controller) *MockGalileo {
	mock := &MockGalileo{ctrl: ctrl}
	mock.recorder = &MockGalileoMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (_m *MockGalileo) EXPECT() *MockGalileoMockRecorder {
	return _m.recorder
}

// AuthenticateIn mocks base method
func (_m *MockGalileo) AuthenticateIn(_param0 context.Context, _param1 ...interface{}) error {
	_s := []interface{}{_param0}
	for _, _x := range _param1 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "AuthenticateIn", _s...)
	ret0, _ := ret[0].(error)
	return ret0
}

// AuthenticateIn indicates an expected call of AuthenticateIn
func (_mr *MockGalileoMockRecorder) AuthenticateIn(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0}, arg1...)
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "AuthenticateIn", reflect.TypeOf((*MockGalileo)(nil).AuthenticateIn), _s...)
}

// AuthenticateOut mocks base method
func (_m *MockGalileo) AuthenticateOut(_param0 context.Context, _param1 string, _param2 ...interface{}) (context.Context, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "AuthenticateOut", _s...)
	ret0, _ := ret[0].(context.Context)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AuthenticateOut indicates an expected call of AuthenticateOut
func (_mr *MockGalileoMockRecorder) AuthenticateOut(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "AuthenticateOut", reflect.TypeOf((*MockGalileo)(nil).AuthenticateOut), _s...)
}

// Endpoint mocks base method
func (_m *MockGalileo) Endpoint(_param0 string) (galileo_go_git.EndpointCfg, error) {
	ret := _m.ctrl.Call(_m, "Endpoint", _param0)
	ret0, _ := ret[0].(galileo_go_git.EndpointCfg)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Endpoint indicates an expected call of Endpoint
func (_mr *MockGalileoMockRecorder) Endpoint(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "Endpoint", reflect.TypeOf((*MockGalileo)(nil).Endpoint), arg0)
}

// Name mocks base method
func (_m *MockGalileo) Name() string {
	ret := _m.ctrl.Call(_m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name
func (_mr *MockGalileoMockRecorder) Name() *gomock.Call {
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "Name", reflect.TypeOf((*MockGalileo)(nil).Name))
}
