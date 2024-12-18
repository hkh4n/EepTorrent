// Code generated by MockGen. DO NOT EDIT.
// Source: eeptorrent/lib/download (interfaces: DownloadManagerInterface)
//
// Generated by this command:
//
//	mockgen -destination=mocks/mock_download_manager.go -package=mocks eeptorrent/lib/download DownloadManagerInterface
//

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	peerprotocol "github.com/go-i2p/go-i2p-bt/peerprotocol"
	gomock "go.uber.org/mock/gomock"
)

// MockDownloadManagerInterface is a mock of DownloadManagerInterface interface.
type MockDownloadManagerInterface struct {
	ctrl     *gomock.Controller
	recorder *MockDownloadManagerInterfaceMockRecorder
	isgomock struct{}
}

// MockDownloadManagerInterfaceMockRecorder is the mock recorder for MockDownloadManagerInterface.
type MockDownloadManagerInterfaceMockRecorder struct {
	mock *MockDownloadManagerInterface
}

// NewMockDownloadManagerInterface creates a new mock instance.
func NewMockDownloadManagerInterface(ctrl *gomock.Controller) *MockDownloadManagerInterface {
	mock := &MockDownloadManagerInterface{ctrl: ctrl}
	mock.recorder = &MockDownloadManagerInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDownloadManagerInterface) EXPECT() *MockDownloadManagerInterfaceMockRecorder {
	return m.recorder
}

// AddPeer mocks base method.
func (m *MockDownloadManagerInterface) AddPeer(peer *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddPeer", peer)
}

// AddPeer indicates an expected call of AddPeer.
func (mr *MockDownloadManagerInterfaceMockRecorder) AddPeer(peer any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddPeer", reflect.TypeOf((*MockDownloadManagerInterface)(nil).AddPeer), peer)
}

// GetAllPeers mocks base method.
func (m *MockDownloadManagerInterface) GetAllPeers() []*peerprotocol.PeerConn {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAllPeers")
	ret0, _ := ret[0].([]*peerprotocol.PeerConn)
	return ret0
}

// GetAllPeers indicates an expected call of GetAllPeers.
func (mr *MockDownloadManagerInterfaceMockRecorder) GetAllPeers() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAllPeers", reflect.TypeOf((*MockDownloadManagerInterface)(nil).GetAllPeers))
}

// GetBlock mocks base method.
func (m *MockDownloadManagerInterface) GetBlock(pieceIndex, offset, length uint32) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBlock", pieceIndex, offset, length)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBlock indicates an expected call of GetBlock.
func (mr *MockDownloadManagerInterfaceMockRecorder) GetBlock(pieceIndex, offset, length any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBlock", reflect.TypeOf((*MockDownloadManagerInterface)(nil).GetBlock), pieceIndex, offset, length)
}

// IsFinished mocks base method.
func (m *MockDownloadManagerInterface) IsFinished() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsFinished")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsFinished indicates an expected call of IsFinished.
func (mr *MockDownloadManagerInterfaceMockRecorder) IsFinished() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsFinished", reflect.TypeOf((*MockDownloadManagerInterface)(nil).IsFinished))
}

// NeedPiecesFrom mocks base method.
func (m *MockDownloadManagerInterface) NeedPiecesFrom(pc *peerprotocol.PeerConn) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NeedPiecesFrom", pc)
	ret0, _ := ret[0].(bool)
	return ret0
}

// NeedPiecesFrom indicates an expected call of NeedPiecesFrom.
func (mr *MockDownloadManagerInterfaceMockRecorder) NeedPiecesFrom(pc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NeedPiecesFrom", reflect.TypeOf((*MockDownloadManagerInterface)(nil).NeedPiecesFrom), pc)
}

// OnBlock mocks base method.
func (m *MockDownloadManagerInterface) OnBlock(index, begin uint32, block []byte) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OnBlock", index, begin, block)
	ret0, _ := ret[0].(error)
	return ret0
}

// OnBlock indicates an expected call of OnBlock.
func (mr *MockDownloadManagerInterfaceMockRecorder) OnBlock(index, begin, block any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnBlock", reflect.TypeOf((*MockDownloadManagerInterface)(nil).OnBlock), index, begin, block)
}

// Progress mocks base method.
func (m *MockDownloadManagerInterface) Progress() float64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Progress")
	ret0, _ := ret[0].(float64)
	return ret0
}

// Progress indicates an expected call of Progress.
func (mr *MockDownloadManagerInterfaceMockRecorder) Progress() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Progress", reflect.TypeOf((*MockDownloadManagerInterface)(nil).Progress))
}

// RemovePeer mocks base method.
func (m *MockDownloadManagerInterface) RemovePeer(peer *peerprotocol.PeerConn) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemovePeer", peer)
}

// RemovePeer indicates an expected call of RemovePeer.
func (mr *MockDownloadManagerInterfaceMockRecorder) RemovePeer(peer any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemovePeer", reflect.TypeOf((*MockDownloadManagerInterface)(nil).RemovePeer), peer)
}

// VerifyPiece mocks base method.
func (m *MockDownloadManagerInterface) VerifyPiece(index uint32) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "VerifyPiece", index)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// VerifyPiece indicates an expected call of VerifyPiece.
func (mr *MockDownloadManagerInterfaceMockRecorder) VerifyPiece(index any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "VerifyPiece", reflect.TypeOf((*MockDownloadManagerInterface)(nil).VerifyPiece), index)
}
