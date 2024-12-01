package i2p

import (
	"github.com/go-i2p/i2pkeys"
	"net"
)

//go:generate mockgen -destination=mocks/mock_i2p_manager.go -package=mocks eeptorrent/lib/i2p I2PManagerInterface
//go:generate mockgen -destination=mocks/mock_stream_session.go -package=mocks eeptorrent/lib/i2p StreamSessionInterface

type I2PManagerInterface interface {
	InitSAM(cfg SAMConfig) error
	CloseSAM()
	GetStreamSession() StreamSessionInterface
}

type StreamSessionInterface interface {
	Dial(network, addr string) (net.Conn, error)
	Listen() (net.Listener, error)
	Addr() i2pkeys.I2PAddr
	Close() error
}
