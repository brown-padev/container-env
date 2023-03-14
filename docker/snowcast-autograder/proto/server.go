package proto

import (
	"fmt"
	"snowcast-autograder/util"
)

const (
	WELCOME_TYPE = iota
	ANNOUNCE_TYPE
	INVALIDCOMMAND_TYPE
)

// type Reply []byte

type Welcome struct {
	NumStations uint16
}

type Announce struct {
	SongName string
}

type InvalidCommand struct {
	ErrorMsg string
}

func (w *Welcome) Serialize() []byte {
	buf := make([]byte, 0, 3)
	buf = append(buf, WELCOME_TYPE)
	buf = append(buf, util.Htons(w.NumStations)...)
	return buf
}

func (w *Welcome) Deserialize(buf []byte) error {
	replyType := uint8(buf[0])
	if replyType != WELCOME_TYPE {
		return fmt.Errorf("welcome reply must have type 0")
	}
	w.NumStations = util.Ntohs(buf[1:])

	return nil
}

func (a *Announce) Serialize() []byte {
	songNameLen := len(a.SongName)
	buf := make([]byte, 0, 2+songNameLen)
	buf = append(buf, ANNOUNCE_TYPE)
	buf = append(buf, uint8(songNameLen))
	buf = append(buf, []byte(a.SongName)...)
	return buf
}

func (a *Announce) Deserialize(buf []byte) error {
	replyType := uint8(buf[0])
	if replyType != ANNOUNCE_TYPE {
		return fmt.Errorf("announce reply must have type 1")
	}

	songNameLen := uint8(buf[1])
	if uint8(len(buf)-2) != songNameLen {
		return fmt.Errorf("song name length and buffer length do not match")
	}
	a.SongName = string(buf[2:])

	return nil
}

func (ic *InvalidCommand) Serialize() []byte {
	msgLen := len(ic.ErrorMsg)
	buf := make([]byte, 0, 2+msgLen)
	buf = append(buf, INVALIDCOMMAND_TYPE)
	buf = append(buf, uint8(msgLen))
	buf = append(buf, []byte(ic.ErrorMsg)...)
	return buf
}

func (ic *InvalidCommand) Deserialize(buf []byte) error {
	replyType := uint8(buf[0])
	if replyType != INVALIDCOMMAND_TYPE {
		return fmt.Errorf("invalid command reply must have type 2")
	}

	msgLen := uint8(buf[1])
	if uint8(len(buf)-2) != msgLen {
		return fmt.Errorf("song name length and buffer length do not match")
	}
	ic.ErrorMsg = string(buf[2:])

	return nil
}
