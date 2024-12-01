package upload

import pp "github.com/go-i2p/go-i2p-bt/peerprotocol"

//go:generate mockgen -destination=mocks/mock_upload_manager.go -package=mocks eeptorrent/lib/upload UploadManagerInterface

type UploadManagerInterface interface {
	AddPeer(peer *pp.PeerConn)
	RemovePeer(peer *pp.PeerConn)
	GetBlock(pieceIndex, offset, length uint32) ([]byte, error)
	VerifyPiece(index uint32) (bool, error)
	GetUploadStats() int64
	Serve()
	Shutdown()
}
