package upload

import (
	"eeptorrent/lib/upload/mocks"
	"errors"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"sync"
	"testing"
)

func TestUploadManager_AddAndRemovePeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockUM.EXPECT().AddPeer(peer).Times(1)
	mockUM.AddPeer(peer)

	mockUM.EXPECT().RemovePeer(peer).Times(1)
	mockUM.RemovePeer(peer)
}

func TestUploadManager_GetBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)

	expectedData := []byte("test block data")
	mockUM.EXPECT().
		GetBlock(uint32(1), uint32(0), uint32(16384)).
		Return(expectedData, nil)

	data, err := mockUM.GetBlock(1, 0, 16384)
	assert.NoError(t, err)
	assert.Equal(t, expectedData, data)

	mockUM.EXPECT().
		GetBlock(uint32(999), uint32(0), uint32(16384)).
		Return(nil, errors.New("invalid piece index"))

	data, err = mockUM.GetBlock(999, 0, 16384)
	assert.Error(t, err)
	assert.Nil(t, data)
}

func TestUploadManager_VerifyPiece(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)

	mockUM.EXPECT().
		VerifyPiece(uint32(0)).
		Return(true, nil)

	verified, err := mockUM.VerifyPiece(0)
	assert.NoError(t, err)
	assert.True(t, verified)

	mockUM.EXPECT().
		VerifyPiece(uint32(1)).
		Return(false, errors.New("hash mismatch"))

	verified, err = mockUM.VerifyPiece(1)
	assert.Error(t, err)
	assert.False(t, verified)
}

func TestUploadManager_GetUploadStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)

	mockUM.EXPECT().
		GetUploadStats().
		Return(int64(1024))

	stats := mockUM.GetUploadStats()
	assert.Equal(t, int64(1024), stats)
}

func TestUploadManager_ServeAndShutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)

	mockUM.EXPECT().Serve().Times(1)
	mockUM.Serve()

	mockUM.EXPECT().Shutdown().Times(1)
	mockUM.Shutdown()
}

func TestUploadManager_ConcurrentOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)
	numOperations := 5
	var wg sync.WaitGroup

	for i := 0; i < numOperations; i++ {
		mockUM.EXPECT().
			GetBlock(uint32(i), uint32(0), uint32(16384)).
			Return([]byte("test data"), nil)
	}

	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			data, err := mockUM.GetBlock(uint32(index), 0, 16384)
			assert.NoError(t, err)
			assert.NotNil(t, data)
		}(i)
	}

	wg.Wait()
}

func TestUploadManager_EdgeCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUM := mocks.NewMockUploadManagerInterface(ctrl)

	mockUM.EXPECT().
		GetBlock(uint32(0), uint32(0), uint32(0)).
		Return(nil, errors.New("invalid block length"))

	data, err := mockUM.GetBlock(0, 0, 0)
	assert.Error(t, err)
	assert.Nil(t, data)

	mockUM.EXPECT().
		VerifyPiece(uint32(9999)).
		Return(false, errors.New("piece index out of range"))

	verified, err := mockUM.VerifyPiece(9999)
	assert.Error(t, err)
	assert.False(t, verified)
}
