package peer

/*
import (
	"testing"

	"eeptorrent/lib/download"

	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockDownloadManager struct {
	mock.Mock
	download.DownloadManager
}

func (m *MockDownloadManager) OnBlock(index uint32, begin uint32, block []byte) error {
	args := m.Called(index, begin, block)
	return args.Error(0)
}

func (m *MockDownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	args := m.Called(pc)
	return args.Bool(0)
}

func (m *MockDownloadManager) VerifyPiece(index uint32) bool {
	args := m.Called(index)
	return args.Bool(0)
}

func (m *MockDownloadManager) BroadcastHave(index uint32) {}

func TestHandleMessageBitField(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{}
	msg := pp.Message{
		Type:     pp.MTypeBitField,
		BitField: pp.NewBitField(4),
	}

	mockDM.On("NeedPiecesFrom", pc).Return(true)
	mockDM.On("OnBlock", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for BitField")
	mockDM.AssertExpectations(t)
}

func TestHandleMessageHave(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{
		BitField: pp.NewBitField(4),
	}
	msg := pp.Message{
		Type:  pp.MTypeHave,
		Index: 2,
	}

	mockDM.On("NeedPiecesFrom", pc).Return(true)
	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for Have")
	assert.True(t, pc.BitField.IsSet(2), "Peer's BitField should be updated")
	mockDM.AssertExpectations(t)
}

func TestHandleMessageChoke(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{
		PeerChoked: false,
	}
	msg := pp.Message{
		Type: pp.MTypeChoke,
	}

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for Choke")
	assert.True(t, pc.PeerChoked, "Peer should be marked as choked")
}

func TestHandleMessageUnchoke(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{
		PeerChoked: true,
	}
	msg := pp.Message{
		Type: pp.MTypeUnchoke,
	}

	mockDM.On("VerifyPiece", mock.Anything).Return(true)
	mockDM.On("BroadcastHave", mock.Anything).Return()

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for Unchoke")
	assert.False(t, pc.PeerChoked, "Peer should be marked as unchoked")
	mockDM.AssertExpectations(t)
}

func TestHandleMessagePiece(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{}
	msg := pp.Message{
		Type:  pp.MTypePiece,
		Index: 1,
		Begin: 0,
		Piece: []byte("testblockdata"),
	}

	mockDM.On("OnBlock", uint32(1), uint32(0), msg.Piece).Return(nil)
	mockDM.On("VerifyPiece", uint32(1)).Return(true)
	mockDM.On("BroadcastHave", uint32(1)).Return()

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for Piece")
	mockDM.AssertExpectations(t)
}

func TestHandleMessageExtendedHandshake(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{}
	extendedHandshake := `d8:completei0e10:incompletei0e13:downloadedi0e12:uploadedi0ee`

	msg := pp.Message{
		Type:            pp.MTypeExtended,
		ExtendedID:      0,
		ExtendedPayload: []byte(extendedHandshake),
	}

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.NoError(t, err, "HandleMessage should not return error for Extended Handshake")
}

func TestHandleMessageInvalidExtended(t *testing.T) {
	// Setup
	mockDM := new(MockDownloadManager)
	pc := &pp.PeerConn{}
	invalidHandshake := `invalid data`

	msg := pp.Message{
		Type:            pp.MTypeExtended,
		ExtendedID:      0,
		ExtendedPayload: []byte(invalidHandshake),
	}

	err := handleMessage(pc, msg, mockDM, &PeerState{})
	assert.Error(t, err, "HandleMessage should return error for invalid Extended Handshake")
}

*/
