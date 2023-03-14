package mock

import (
	"errors"
	"fmt"
	"io"
	"net"
	"snowcast-autograder/proto"
	"snowcast-autograder/util"
	"syscall"
	"time"
)

type Control struct {
	conn         net.Conn // server connection
	ServerName   string
	ServerPort   uint16
	ListenerPort uint16

	timeout time.Duration // time for socket opearions to timeout; default is TIMEOUT (250ms)
}

// Initializes a new mock control instance with the specified server.
func NewControl(serverName string, serverPort, listenerPort uint16) (*Control, error) {
	server := fmt.Sprintf("%s:%v", serverName, serverPort)
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server %s: %w", server, err)
	}
	mc := Control{
		conn:         conn,
		ServerName:   serverName,
		ServerPort:   serverPort,
		ListenerPort: listenerPort,
		timeout:      TIMEOUT,
	}
	return &mc, nil
}

// Get IP of connection.
func (mc *Control) ConnIP() net.IP {
	addr := mc.conn.LocalAddr().(*net.TCPAddr)
	return addr.IP
}

// Sets the control's timeout value.
func (mc *Control) SetTimeout(d time.Duration) {
	mc.timeout = d
}

// Wrapper around io.ReadFull on the connection, setting timeout.
func (mc *Control) ReadTimeout(buf []byte) (int, error) {
	mc.conn.SetReadDeadline(time.Time.Add(time.Now(), mc.timeout))
	return io.ReadFull(mc.conn, buf)
}

// Wrapper around conn.WriteTimeout(buf), setting timeout.
func (mc *Control) WriteTimeout(buf []byte) (int, error) {
	mc.conn.SetWriteDeadline(time.Time.Add(time.Now(), mc.timeout))
	return mc.conn.Write(buf)
}

// Returns true if the connection is closed, false otherwise.
func (mc *Control) ConnClosed() bool {
	mc.conn.SetReadDeadline(time.Time.Add(time.Now(), mc.timeout))
	// if successfully closed, should've read all to EOF (which is err == nil); if server closed,
	// should've gotten ECONNRESET
	if _, err := io.ReadAll(mc.conn); err == nil || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return false
}

func (mc *Control) Quit() error {
	if err := mc.conn.Close(); err != nil {
		return err
	}
	return nil
}

// Sends a Hello message to the server.
func (mc *Control) Hello() error {
	h := proto.Hello{UDPPort: mc.ListenerPort}
	buf := h.Serialize()
	if _, err := mc.WriteTimeout(buf); err != nil {
		return fmt.Errorf("failed to send hello message: %w", err)
	}

	return nil
}

// Sends a SetStation message with the desired station number to the server.
func (mc *Control) SetStation(stationNumber uint16) error {
	ss := proto.SetStation{StationNumber: stationNumber}
	buf := ss.Serialize()
	if _, err := mc.WriteTimeout(buf); err != nil {
		return fmt.Errorf("failed to send setstation message: %w", err)
	}

	return nil
}

// Sends an invalid command with the specified reply type to the server.
func (mc *Control) InvalidCommand(cmdType uint8) error {
	if _, err := mc.WriteTimeout([]byte{cmdType}); err != nil {
		return fmt.Errorf("failed to send an invalid command: %w", err)
	}

	return nil
}

// Attempts to parse a Welcome message from the server; errors if incorrect type.
func (mc *Control) GetWelcome() (proto.Welcome, error) {
	w := proto.Welcome{}

	welcomeSize := 3
	buf := make([]byte, welcomeSize)
	if _, err := mc.ReadTimeout(buf); err != nil {
		return w, fmt.Errorf("failed to read message from conn: %w", err)
	}
	replyType := uint8(buf[0])
	if replyType != proto.WELCOME_TYPE {
		return w, fmt.Errorf("welcome reply must have type 0")
	}
	w.NumStations = util.Ntohs(buf[1:])
	return w, nil
}

// Attempts to parse an Announce message; errors if incorrect type.
func (mc *Control) GetAnnounce() (proto.Announce, error) {
	a := proto.Announce{}

	// first, read reply type
	buf := make([]byte, 1)
	if _, err := mc.ReadTimeout(buf); err != nil {
		return a, fmt.Errorf("failed to read message from conn: %w", err)
	}
	replyType := uint8(buf[0])
	if replyType != proto.ANNOUNCE_TYPE {
		return a, fmt.Errorf("invalid type (expected %v, got %v)", proto.ANNOUNCE_TYPE, replyType)
	}

	// if valid, read length
	if _, err := mc.ReadTimeout(buf); err != nil {
		return a, fmt.Errorf("failed to read message from conn: %w", err)
	}
	songNameLen := uint8(buf[0])
	buf = make([]byte, songNameLen)
	if _, err := mc.ReadTimeout(buf); err != nil {
		return a, fmt.Errorf("could not read full song name: %w", err)
	}
	a.SongName = string(buf)

	return a, nil
}

// Attempts to parse an InvalidCommand message; errors if incorrect type.
func (mc *Control) GetInvalidCommand() (proto.InvalidCommand, error) {
	ic := proto.InvalidCommand{}

	// first, read reply type
	buf := make([]byte, 1)
	if _, err := mc.ReadTimeout(buf); err != nil {
		return ic, fmt.Errorf("failed to read message from conn: %w", err)
	}
	replyType := uint8(buf[0])
	if replyType != proto.INVALIDCOMMAND_TYPE {
		return ic, fmt.Errorf(
			"invalid type (expected %v, got %v)",
			proto.INVALIDCOMMAND_TYPE,
			replyType,
		)
	}

	// if valid, read length
	if _, err := mc.ReadTimeout(buf); err != nil {
		return ic, fmt.Errorf("failed to read message from conn: %w", err)
	}
	msgLen := uint8(buf[0])
	buf = make([]byte, msgLen)
	if _, err := mc.ReadTimeout(buf); err != nil {
		return ic, fmt.Errorf("could not read full song name: %w", err)
	}
	ic.ErrorMsg = string(buf)

	return ic, nil
}
