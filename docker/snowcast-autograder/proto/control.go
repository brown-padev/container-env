package proto

import (
	"fmt"
	"snowcast-autograder/util"
)

const (
	HELLO_TYPE = iota
	SETSTATION_TYPE
)

type Hello struct {
	UDPPort uint16
}

type SetStation struct {
	StationNumber uint16
}

func (h *Hello) Size() uint {
	return 3
}

func (h *Hello) Serialize() []byte {
	buf := make([]byte, 0, 3)
	buf = append(buf, HELLO_TYPE)
	buf = append(buf, util.Htons(h.UDPPort)...)
	return buf
}

func (h *Hello) Deserialize(buf []byte) error {
	commandType := uint8(buf[0])
	if commandType != HELLO_TYPE {
		return fmt.Errorf("hello command must have type %v", HELLO_TYPE)
	}
	h.UDPPort = util.Ntohs(buf[1:])

	return nil
}

func (ss *SetStation) Serialize() []byte {
	buf := make([]byte, 0, 3)
	buf = append(buf, SETSTATION_TYPE)
	buf = append(buf, util.Htons(ss.StationNumber)...)
	return buf
}

func (ss *SetStation) Deserialize(buf []byte) error {
	commandType := uint8(buf[0])
	if commandType != SETSTATION_TYPE {
		return fmt.Errorf("set station command must have type %v", SETSTATION_TYPE)
	}
	ss.StationNumber = util.Ntohs(buf[1:])

	return nil
}
