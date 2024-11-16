package i2p

import (
	"fmt"
	"github.com/go-i2p/sam3"
	"os"
	"strconv"
)

var GlobalSAM *sam3.SAM
var GlobalStreamSession *sam3.StreamSession

func InitSAM() error {
	var err error
	GlobalSAM, err = sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		return fmt.Errorf("Failed to create global SAM session: %v", err)
	}

	globalKeys, err := GlobalSAM.NewKeys()
	if err != nil {
		return fmt.Errorf("Failed to generate keys for global SAM session: %v", err)
	}

	options := []string{
		"inbound.length=1",
		"outbound.length=1",
		"inbound.quantity=3",
		"outbound.quantity=3",
		"inbound.backupQuantity=1",
		"outbound.backupQuantity=1",
		"inbound.lengthVariance=0",
		"outbound.lengthVariance=0",
	}

	globalSessionName := fmt.Sprintf("global-session-%d", os.Getpid())
	GlobalStreamSession, err = GlobalSAM.NewStreamSessionWithSignature(
		globalSessionName,
		globalKeys,
		options,
		strconv.Itoa(7),
	)
	if err != nil {
		return fmt.Errorf("Failed to create global SAM stream session: %v", err)
	}

	return nil
}

func CloseSAM() {
	if GlobalStreamSession != nil {
		GlobalStreamSession.Close()
	}
	if GlobalSAM != nil {
		GlobalSAM.Close()
	}
}
