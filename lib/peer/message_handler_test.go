package peer

import (
	"eeptorrent/lib/download"
	"errors"
	"github.com/go-i2p/go-i2p-bt/bencode"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

// MockDownloadManager is a mock implementation of DownloadManager
type MockDownloadManager struct {
	mock.Mock
	Bitfield     *pp.BitField
	Pieces       []*download.Piece
	IsFinishedFn func() bool
}

func (m *MockDownloadManager) IsFinished() bool {
	if m.IsFinishedFn != nil {
		return m.IsFinishedFn()
	}
	args := m.Called()
	return args.Bool(0)
}

func (m *MockDownloadManager) OnBlock(pieceIndex, begin uint32, block []byte) error {
	args := m.Called(pieceIndex, begin, block)
	return args.Error(0)
}

func (m *MockDownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	args := m.Called(pc)
	return args.Bool(0)
}

func (m *MockDownloadManager) GetBlock(pieceIndex, begin, length uint32) ([]byte, error) {
	args := m.Called(pieceIndex, begin, length)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockDownloadManager) VerifyPiece(pieceIndex uint32) (bool, error) {
	args := m.Called(pieceIndex)
	return args.Bool(0), args.Error(1)
}

func (m *MockDownloadManager) GetPieceLength(pieceIndex uint32) int64 {
	args := m.Called(pieceIndex)
	return int64(args.Int(0))
}

func TestHandleMessage_BitField(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Setup Bitfield
	mockPeer.BitField = NewBitfield(100)

	// Create a BitField message
	msg := pp.Message{
		Type:     pp.MTypeBitField,
		BitField: NewBitfield(100),
	}

	// Mock NeedPiecesFrom
	dm.On("NeedPiecesFrom", mockPeer).Return(true)

	// Mock SendInterested and requestNextBlock
	mockPeer.On("SendInterested").Return(nil)

	// Mock requestNextBlock behavior
	// For simplicity, assume no error
	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	mockPeer.AssertCalled(t, "SendInterested")
}

func TestHandleMessage_Have(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Setup Bitfield
	mockPeer.BitField = NewBitfield(100)

	// Create a Have message
	pieceIndex := uint32(5)
	msg := pp.Message{
		Type:  pp.MTypeHave,
		Index: pieceIndex,
	}

	// Mock NeedPiecesFrom
	dm.On("NeedPiecesFrom", mockPeer).Return(true)

	// Mock SendInterested
	mockPeer.On("SendInterested").Return(nil)

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	assert.True(t, mockPeer.BitField.IsSet(pieceIndex), "BitField should be updated with the new piece")
	mockPeer.AssertCalled(t, "SendInterested")
}

func TestHandleMessage_Choke(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Initially unchoked
	mockPeer.PeerChoked = false

	// Create a Choke message
	msg := pp.Message{
		Type: pp.MTypeChoke,
	}

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	assert.True(t, mockPeer.PeerChoked, "Peer should be marked as choked")
	assert.False(t, pm.Peers[mockPeer].RequestPending, "RequestPending should be reset")
	assert.Equal(t, int32(0), pm.Peers[mockPeer].PendingRequests, "PendingRequests should be reset")
}

func TestHandleMessage_Unchoke(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Initially choked
	mockPeer.PeerChoked = true

	// Create an Unchoke message
	msg := pp.Message{
		Type: pp.MTypeUnchoke,
	}

	// Mock SendInterested and requestNextBlock
	// Assuming requestNextBlock will be called without error
	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	assert.False(t, mockPeer.PeerChoked, "Peer should be marked as unchoked")
	// Depending on requestNextBlock implementation, further assertions can be added
}

func TestHandleMessage_Piece(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Setup initial state
	pm.peerStats[mockPeer].PendingRequests = 1
	pm.peerStats[mockPeer].bytesDownloaded = 0
	pm.Peers[mockPeer].RequestPending = true

	// Create a Piece message
	pieceIndex := uint32(2)
	begin := uint32(0)
	blockData := []byte("test block data")
	msg := pp.Message{
		Type:  pp.MTypePiece,
		Index: pieceIndex,
		Begin: begin,
		Piece: blockData,
	}

	// Mock OnBlock
	dm.On("OnBlock", pieceIndex, begin, blockData).Return(nil)

	// Mock VerifyPiece
	dm.On("VerifyPiece", pieceIndex).Return(true, nil)

	// Mock SendHave
	mockPeer.On("SendHave", pieceIndex).Return(nil)

	// Mock UpdatePeerStats
	pm.On("UpdatePeerStats", mockPeer, int64(len(blockData)), 0).Return()

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	assert.False(t, pm.Peers[mockPeer].RequestPending, "RequestPending should be reset after receiving a piece")
	assert.Equal(t, int32(0), pm.Peers[mockPeer].PendingRequests, "PendingRequests should be decremented")
	dm.AssertCalled(t, "OnBlock", pieceIndex, begin, blockData)
	dm.AssertCalled(t, "VerifyPiece", pieceIndex)
	mockPeer.AssertCalled(t, "SendHave", pieceIndex)
}

func TestHandleMessage_ExtendedHandshake(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Create an Extended Handshake message
	extendedID := uint8(0)
	remoteHandshake := ExtendedHandshakeMsg{
		V: "TestClient 1.0",
		M: map[string]uint8{
			"ut_metadata": 1,
		},
	}
	payload, err := bencode.EncodeBytes(remoteHandshake)
	assert.NoError(t, err)

	msg := pp.Message{
		Type:            pp.MTypeExtended,
		ExtendedID:      extendedID,
		ExtendedPayload: payload,
	}

	// Create expected handshake to send
	expectedHandshake := ExtendedHandshakeMsg{
		V: "EepTorrent 0.0.0",
		M: map[string]uint8{
			"ut_metadata": 1,
		},
	}

	// Mock SendExtHandshakeMsg
	mockPeer.On("SendExtHandshakeMsg", expectedHandshake).Return(nil)

	err = handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	mockPeer.AssertCalled(t, "SendExtHandshakeMsg", expectedHandshake)
}

func TestHandleMessage_UnknownMessage(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Create an unknown message type
	msg := pp.Message{
		Type: pp.MTypeReserved, // Assuming MTypeReserved is not handled
	}

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err, "Unknown message types should not cause errors")
}

func TestHandleMessage_Request(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Create a Request message
	pieceIndex := uint32(3)
	begin := uint32(0)
	length := uint32(16384)
	msg := pp.Message{
		Type:   pp.MTypeRequest,
		Index:  pieceIndex,
		Begin:  begin,
		Length: length,
	}

	// Mock GetBlock
	blockData := []byte("requested block data")
	dm.On("GetBlock", pieceIndex, begin, length).Return(blockData, nil)

	// Mock SendPiece
	mockPeer.On("SendPiece", pieceIndex, begin, blockData).Return(nil)

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	dm.AssertCalled(t, "GetBlock", pieceIndex, begin, length)
	mockPeer.AssertCalled(t, "SendPiece", pieceIndex, begin, blockData)
}

func TestHandleMessage_Request_GetBlockError(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Create a Request message
	pieceIndex := uint32(3)
	begin := uint32(0)
	length := uint32(16384)
	msg := pp.Message{
		Type:   pp.MTypeRequest,
		Index:  pieceIndex,
		Begin:  begin,
		Length: length,
	}

	// Mock GetBlock to return an error
	dm.On("GetBlock", pieceIndex, begin, length).Return(nil, errors.New("block not available"))

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.Error(t, err, "handleMessage should return an error if GetBlock fails")
	dm.AssertCalled(t, "GetBlock", pieceIndex, begin, length)
}

func TestHandleMessage_NotInterested(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Create a NotInterested message
	msg := pp.Message{
		Type: pp.MTypeNotInterested,
	}

	// Mock OnPeerNotInterested
	// Since OnPeerNotInterested doesn't have a return value, we can verify internal state

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err)
	assert.False(t, pm.Peers[mockPeer].RequestPending, "RequestPending should remain unaffected")
}

func TestHandleMessage_Piece_VerifyFailure(t *testing.T) {
	dm := new(MockDownloadManager)
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Setup initial state
	pm.peerStats[mockPeer].PendingRequests = 1
	pm.peerStats[mockPeer].bytesDownloaded = 0
	pm.Peers[mockPeer].RequestPending = true

	// Create a Piece message
	pieceIndex := uint32(2)
	begin := uint32(0)
	blockData := []byte("test block data")
	msg := pp.Message{
		Type:  pp.MTypePiece,
		Index: pieceIndex,
		Begin: begin,
		Piece: blockData,
	}

	// Mock OnBlock
	dm.On("OnBlock", pieceIndex, begin, blockData).Return(nil)

	// Mock VerifyPiece to fail
	dm.On("VerifyPiece", pieceIndex).Return(false, nil)

	err := handleMessage(mockPeer, msg, dm, pm.Peers[mockPeer], pm)

	assert.NoError(t, err, "handleMessage should not return error even if verification fails")
	dm.AssertCalled(t, "OnBlock", pieceIndex, begin, blockData)
	dm.AssertCalled(t, "VerifyPiece", pieceIndex)
	mockPeer.AssertNotCalled(t, "SendHave", pieceIndex)
}
