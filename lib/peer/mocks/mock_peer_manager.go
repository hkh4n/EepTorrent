// Code generated by MockGen. DO NOT EDIT.
// Source: eeptorrent/lib/peer (interfaces: PeerManagerInterface)
//
// Generated by this command:
//
//	mockgen -destination=mocks/mock_peer_manager.go -package=mocks eeptorrent/lib/peer PeerManagerInterface
//

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	peerprotocol "github.com/go-i2p/go-i2p-bt/peerprotocol"
	gomock "go.uber.org/mock/gomock"
)

// MockPeerManagerInterface is a mock of PeerManagerInterface interface.
type MockPeerManagerInterface struct {
	ctrl     *gomock.Controller
	recorder *MockPeerManagerInterfaceMockRecorder
	isgomock struct{}
}

// MockPeerManagerInterfaceMockRecorder is the mock recorder for MockPeerManagerInterface.
type MockPeerManagerInterfaceMockRecorder struct {
	mock *MockPeerManagerInterface
}

// NewMockPeerManagerInterface creates a new mock instance.
func NewMockPeerManagerInterface(ctrl *gomock.Controller) *MockPeerManagerInterface {
	mock := &MockPeerManagerInterface{ctrl: ctrl}
	mock.recorder = &MockPeerManagerInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPeerManagerInterface) EXPECT() *MockPeerManagerInterfaceMockRecorder {
	return m.recorder
}

// AddPeer mocks base method.
func (m *MockPeerManagerInterface) AddPeer(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddPeer", pc)
}

// AddPeer indicates an expected call of AddPeer.
func (mr *MockPeerManagerInterfaceMockRecorder) AddPeer(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddPeer", reflect.TypeOf((*MockPeerManagerInterface)(nil).AddPeer), pc)
}

// HandleSeeding mocks base method.
func (m *MockPeerManagerInterface) HandleSeeding() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "HandleSeeding")
}

// HandleSeeding indicates an expected call of HandleSeeding.
func (mr *MockPeerManagerInterfaceMockRecorder) HandleSeeding() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleSeeding", reflect.TypeOf((*MockPeerManagerInterface)(nil).HandleSeeding))
}

// OnPeerChoke mocks base method.
func (m *MockPeerManagerInterface) OnPeerChoke(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "OnPeerChoke", pc)
}

// OnPeerChoke indicates an expected call of OnPeerChoke.
func (mr *MockPeerManagerInterfaceMockRecorder) OnPeerChoke(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnPeerChoke", reflect.TypeOf((*MockPeerManagerInterface)(nil).OnPeerChoke), pc)
}

// OnPeerInterested mocks base method.
func (m *MockPeerManagerInterface) OnPeerInterested(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "OnPeerInterested", pc)
}

// OnPeerInterested indicates an expected call of OnPeerInterested.
func (mr *MockPeerManagerInterfaceMockRecorder) OnPeerInterested(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnPeerInterested", reflect.TypeOf((*MockPeerManagerInterface)(nil).OnPeerInterested), pc)
}

// OnPeerNotInterested mocks base method.
func (m *MockPeerManagerInterface) OnPeerNotInterested(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "OnPeerNotInterested", pc)
}

// OnPeerNotInterested indicates an expected call of OnPeerNotInterested.
func (mr *MockPeerManagerInterfaceMockRecorder) OnPeerNotInterested(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnPeerNotInterested", reflect.TypeOf((*MockPeerManagerInterface)(nil).OnPeerNotInterested), pc)
}

// OnPeerUnchoke mocks base method.
func (m *MockPeerManagerInterface) OnPeerUnchoke(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "OnPeerUnchoke", pc)
}

// OnPeerUnchoke indicates an expected call of OnPeerUnchoke.
func (mr *MockPeerManagerInterfaceMockRecorder) OnPeerUnchoke(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnPeerUnchoke", reflect.TypeOf((*MockPeerManagerInterface)(nil).OnPeerUnchoke), pc)
}

// RemovePeer mocks base method.
func (m *MockPeerManagerInterface) RemovePeer(pc *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemovePeer", pc)
}

// RemovePeer indicates an expected call of RemovePeer.
func (mr *MockPeerManagerInterfaceMockRecorder) RemovePeer(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemovePeer", reflect.TypeOf((*MockPeerManagerInterface)(nil).RemovePeer), pc)
}

// Shutdown mocks base method.
func (m *MockPeerManagerInterface) Shutdown() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Shutdown")
}

// Shutdown indicates an expected call of Shutdown.
func (mr *MockPeerManagerInterfaceMockRecorder) Shutdown() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Shutdown", reflect.TypeOf((*MockPeerManagerInterface)(nil).Shutdown))
}

// UpdatePeerStats mocks base method.
func (m *MockPeerManagerInterface) UpdatePeerStats(pc *peerprotocol.PeerConn, bytesDownloaded, bytesUploaded int64) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "UpdatePeerStats", pc, bytesDownloaded, bytesUploaded)
}

// UpdatePeerStats indicates an expected call of UpdatePeerStats.
func (mr *MockPeerManagerInterfaceMockRecorder) UpdatePeerStats(pc, bytesDownloaded, bytesUploaded any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePeerStats", reflect.TypeOf((*MockPeerManagerInterface)(nil).UpdatePeerStats), pc, bytesDownloaded, bytesUploaded)
}
