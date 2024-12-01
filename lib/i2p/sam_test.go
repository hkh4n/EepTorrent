package i2p_test

import (
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/i2p/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"net"
	"testing"
)

func TestSAMInitialization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := mocks.NewMockI2PManagerInterface(ctrl)
	mockSession := mocks.NewMockStreamSessionInterface(ctrl)

	cfg := i2p.SAMConfig{
		InboundLength:          1,
		OutboundLength:         1,
		InboundQuantity:        3,
		OutboundQuantity:       3,
		InboundBackupQuantity:  1,
		OutboundBackupQuantity: 1,
		InboundLengthVariance:  0,
		OutboundLengthVariance: 0,
	}

	mockManager.EXPECT().
		InitSAM(cfg).
		Return(nil)

	mockManager.EXPECT().
		GetStreamSession().
		Return(mockSession)

	err := mockManager.InitSAM(cfg)
	assert.NoError(t, err)

	session := mockManager.GetStreamSession()
	assert.NotNil(t, session)

	mockConn := &net.TCPConn{}
	mockSession.EXPECT().
		Dial("tcp", "test.i2p").
		Return(mockConn, nil)

	conn, err := session.Dial("tcp", "test.i2p")
	assert.NoError(t, err)
	assert.NotNil(t, conn)
}

func TestSAMClosing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := mocks.NewMockI2PManagerInterface(ctrl)
	mockSession := mocks.NewMockStreamSessionInterface(ctrl)

	mockManager.EXPECT().
		GetStreamSession().
		Return(mockSession)

	mockSession.EXPECT().
		Close().
		Return(nil)

	mockManager.EXPECT().
		CloseSAM().
		Times(1)

	session := mockManager.GetStreamSession()
	assert.NotNil(t, session)

	err := session.Close()
	assert.NoError(t, err)

	mockManager.CloseSAM()
}

func TestStreamSessionListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSession := mocks.NewMockStreamSessionInterface(ctrl)
	mockListener := &net.TCPListener{}

	mockSession.EXPECT().
		Listen().
		Return(mockListener, nil)

	listener, err := mockSession.Listen()
	assert.NoError(t, err)
	assert.NotNil(t, listener)
}

type mockConn struct {
	net.Conn
}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}
