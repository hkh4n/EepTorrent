package download_test

import (
	"bytes"
	"eeptorrent/lib/download/mocks"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestOnBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)

	// Test valid block
	validBlock := []byte("test block data")
	mockDM.EXPECT().
		OnBlock(uint32(1), uint32(0), validBlock).
		Return(nil)

	err := mockDM.OnBlock(1, 0, validBlock)
	assert.NoError(t, err)

	// Test nil block - set up expectation
	mockDM.EXPECT().
		OnBlock(uint32(1), uint32(0), nil).
		Return(assert.AnError)

	err = mockDM.OnBlock(1, 0, nil)
	assert.Error(t, err)
}

func TestPeerManagement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockDM.EXPECT().
		AddPeer(peer).
		Times(1)

	mockDM.AddPeer(peer)

	mockDM.EXPECT().
		GetAllPeers().
		Return([]*pp.PeerConn{peer})

	peers := mockDM.GetAllPeers()
	assert.Len(t, peers, 1)
	assert.Equal(t, peer, peers[0])

	mockDM.EXPECT().
		RemovePeer(peer).
		Times(1)

	mockDM.RemovePeer(peer)
}

func TestPieceVerification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)

	mockDM.EXPECT().
		VerifyPiece(uint32(0)).
		Return(true, nil)

	verified, err := mockDM.VerifyPiece(0)
	assert.NoError(t, err)
	assert.True(t, verified)

	mockDM.EXPECT().
		VerifyPiece(uint32(1)).
		Return(false, nil)

	verified, err = mockDM.VerifyPiece(1)
	assert.NoError(t, err)
	assert.False(t, verified)
}

func TestDownloadProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)

	// Test progress calculation
	mockDM.EXPECT().
		Progress().
		Return(float64(50.0)) // 50% complete

	progress := mockDM.Progress()
	assert.Equal(t, float64(50.0), progress)

	mockDM.EXPECT().
		IsFinished().
		Return(false)

	finished := mockDM.IsFinished()
	assert.False(t, finished)
}

func TestPieceRequirements(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)
	peer := &pp.PeerConn{
		BitField: pp.NewBitField(10),
	}

	mockDM.EXPECT().
		NeedPiecesFrom(peer).
		Return(true)

	needsPieces := mockDM.NeedPiecesFrom(peer)
	assert.True(t, needsPieces)

	mockDM.EXPECT().
		NeedPiecesFrom(peer).
		Return(false)

	needsPieces = mockDM.NeedPiecesFrom(peer)
	assert.False(t, needsPieces)
}

func TestBlockOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDM := mocks.NewMockDownloadManagerInterface(ctrl)

	expectedBlock := []byte("test block data")
	mockDM.EXPECT().
		GetBlock(uint32(0), uint32(0), uint32(16)).
		Return(expectedBlock, nil)

	block, err := mockDM.GetBlock(0, 0, 16)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(expectedBlock, block))

	mockDM.EXPECT().
		GetBlock(uint32(999), uint32(0), uint32(16)).
		Return(nil, assert.AnError)

	block, err = mockDM.GetBlock(999, 0, 16)
	assert.Error(t, err)
	assert.Nil(t, block)
}
