// Code generated by MockGen. DO NOT EDIT.
// Source: eeptorrent/lib/i2p (interfaces: StreamSessionInterface)
//
// Generated by this command:
//
//	mockgen -destination=mocks/mock_stream_session.go -package=mocks eeptorrent/lib/i2p StreamSessionInterface
//

// Package mocks is a generated GoMock package.
package mocks

import (
	net "net"
	reflect "reflect"

	i2pkeys "github.com/go-i2p/i2pkeys"
	gomock "go.uber.org/mock/gomock"
)

// MockStreamSessionInterface is a mock of StreamSessionInterface interface.
type MockStreamSessionInterface struct {
	ctrl     *gomock.Controller
	recorder *MockStreamSessionInterfaceMockRecorder
	isgomock struct{}
}

// MockStreamSessionInterfaceMockRecorder is the mock recorder for MockStreamSessionInterface.
type MockStreamSessionInterfaceMockRecorder struct {
	mock *MockStreamSessionInterface
}

// NewMockStreamSessionInterface creates a new mock instance.
func NewMockStreamSessionInterface(ctrl *gomock.Controller) *MockStreamSessionInterface {
	mock := &MockStreamSessionInterface{ctrl: ctrl}
	mock.recorder = &MockStreamSessionInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamSessionInterface) EXPECT() *MockStreamSessionInterfaceMockRecorder {
	return m.recorder
}

// Addr mocks base method.
func (m *MockStreamSessionInterface) Addr() i2pkeys.I2PAddr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Addr")
	ret0, _ := ret[0].(i2pkeys.I2PAddr)
	return ret0
}

// Addr indicates an expected call of Addr.
func (mr *MockStreamSessionInterfaceMockRecorder) Addr() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Addr", reflect.TypeOf((*MockStreamSessionInterface)(nil).Addr))
}

// Close mocks base method.
func (m *MockStreamSessionInterface) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockStreamSessionInterfaceMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockStreamSessionInterface)(nil).Close))
}

// Dial mocks base method.
func (m *MockStreamSessionInterface) Dial(network, addr string) (net.Conn, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Dial", network, addr)
	ret0, _ := ret[0].(net.Conn)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Dial indicates an expected call of Dial.
func (mr *MockStreamSessionInterfaceMockRecorder) Dial(network, addr any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Dial", reflect.TypeOf((*MockStreamSessionInterface)(nil).Dial), network, addr)
}

// Listen mocks base method.
func (m *MockStreamSessionInterface) Listen() (net.Listener, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Listen")
	ret0, _ := ret[0].(net.Listener)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Listen indicates an expected call of Listen.
func (mr *MockStreamSessionInterfaceMockRecorder) Listen() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Listen", reflect.TypeOf((*MockStreamSessionInterface)(nil).Listen))
}