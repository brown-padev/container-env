// mock classes/methods

package mock

import (
	"fmt"
	"io"
	"net"
	"os"
	"snowcast-autograder/proto"
	"snowcast-autograder/util"
	"time"
)

var (
	TIMEOUT = 250 * time.Millisecond
)

type Station struct {
	ID        int
	File      *os.File
	Listeners []net.Addr
}

type Server struct {
	Port     uint16
	Stations []Station             // list of stations
	listener net.TCPListener       // listener port for incoming connections
	conns    map[net.Addr]net.Conn // map of connections
	timeout  time.Duration         // time for socket operations to timeout; default is TIMEOUT (250ms)
}

// Creates a mock server listening on the specific port with the files as stations.
func NewServer(port uint16, files []string) (*Server, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("pass in at least 1 file")
	}

	server := fmt.Sprintf("localhost:%v", port)
	ln, err := net.Listen("tcp", server)
	if err != nil {
		return nil, fmt.Errorf("failed to listen at address %s: %w", server, err)
	}
	tcp_ln, _ := ln.(*net.TCPListener)

	stations := make([]Station, len(files))
	for i, f := range files {
		file, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		stations[i] = Station{
			ID:   i,
			File: file,
		}
	}

	ms := Server{
		Port:     port,
		Stations: stations,
		listener: *tcp_ln,
		conns:    map[net.Addr]net.Conn{},
		timeout:  TIMEOUT,
	}

	return &ms, nil
}

// Sets the server's timeout value.
func (ms *Server) SetTimeout(d time.Duration) {
	ms.timeout = d
}

// Wrapper around io.ReadFull on the connection, setting timeout.
func (ms *Server) ReadTimeout(conn net.Conn, buf []byte) (int, error) {
	conn.SetReadDeadline(time.Time.Add(time.Now(), ms.timeout))
	return io.ReadFull(conn, buf)
}

// Wrapper around conn.WriteTimeout(buf), setting timeout.
func (ms *Server) WriteTimeout(conn net.Conn, buf []byte) (int, error) {
	conn.SetWriteDeadline(time.Time.Add(time.Now(), ms.timeout))
	return conn.Write(buf)
}

// Wrapper around conn.Write(buf), given an addr.
func (ms *Server) WriteAddr(addr net.Addr, buf []byte) (int, error) {
	conn, ok := ms.conns[addr]
	if !ok {
		return 0, fmt.Errorf("connection from %v does not exist", addr)
	}

	return ms.WriteTimeout(conn, buf)
}

// Accepts a client connection, and returns the address of the client.
func (ms *Server) Accept(timeout time.Duration) (net.Addr, error) {
	if timeout > 0 {
		ms.listener.SetDeadline(time.Time.Add(time.Now(), timeout))
	} else {
		ms.listener.SetDeadline(time.Time{})
	}
	conn, err := ms.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("failed to accept connection: %w", err)
	}

	// super scuffed way to get port
	addr := conn.RemoteAddr()
	ms.conns[addr] = conn

	return addr, nil
}

func (ms *Server) Quit() error {
	if err := ms.listener.Close(); err != nil {
		return err
	}
	for _, c := range ms.conns {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Sends a Welcome message to the desired connection.
func (ms *Server) Welcome(addr net.Addr) error {
	conn, ok := ms.conns[addr]
	if !ok {
		return fmt.Errorf("connection from %v does not exist", addr)
	}

	w := proto.Welcome{NumStations: uint16(len(ms.Stations))}
	buf := w.Serialize()
	if _, err := ms.WriteTimeout(conn, buf); err != nil {
		return fmt.Errorf("failed to send welcome message: %w", err)
	}

	return nil
}

// Sends an Announce message to the desired connection with the specified song name.
func (ms *Server) Announce(addr net.Addr, songName string) error {
	conn, ok := ms.conns[addr]
	if !ok {
		return fmt.Errorf("connection from %v does not exist", addr)
	}

	a := proto.Announce{SongName: songName}
	buf := a.Serialize()
	if _, err := ms.WriteTimeout(conn, buf); err != nil {
		return fmt.Errorf("failed to send announce message: %w", err)
	}

	return nil
}

// Sends an InvalidCommand message to the desired connection with the specified error message.
func (ms *Server) InvalidCommand(addr net.Addr, errMsg string) error {
	conn, ok := ms.conns[addr]
	if !ok {
		return fmt.Errorf("connection from %v does not exist", addr)
	}

	ic := proto.InvalidCommand{ErrorMsg: errMsg}
	buf := ic.Serialize()
	if _, err := ms.WriteTimeout(conn, buf); err != nil {
		return fmt.Errorf("failed to send invalid command message: %w", err)
	}

	return nil
}

// Sends an invalid reply with the specified type to the desired connection.
func (ms *Server) InvalidReply(addr net.Addr, replyType uint8) error {
	conn, ok := ms.conns[addr]
	if !ok {
		return fmt.Errorf("connection from port %v does not exist", addr)
	}

	if _, err := ms.WriteTimeout(conn, []byte{replyType}); err != nil {
		return fmt.Errorf("failed to send an invalid reply: %w", err)
	}

	return nil
}

// Attempts to parse a Hello message from the address's connection; errors if not a Hello message.
func (ms *Server) GetHello(addr net.Addr) (proto.Hello, error) {
	h := proto.Hello{}

	conn, ok := ms.conns[addr]
	if !ok {
		return h, fmt.Errorf("connection from port %v does not exist", addr)
	}
	helloSize := 3
	buf := make([]byte, helloSize)
	if _, err := ms.ReadTimeout(conn, buf); err != nil {
		return h, fmt.Errorf("failed to read message from conn: %w", err)
	}
	cmdType := uint8(buf[0])
	if cmdType != proto.HELLO_TYPE {
		return h, fmt.Errorf("invalid cmd type (expected %v, got %v)", proto.HELLO_TYPE, cmdType)
	}
	h.UDPPort = util.Ntohs(buf[1:])
	return h, nil
}

// Attempts to parse a SetStation message; errors if incorrect type.
func (ms *Server) GetSetStation(addr net.Addr) (proto.SetStation, error) {
	ss := proto.SetStation{}

	conn, ok := ms.conns[addr]
	if !ok {
		return ss, fmt.Errorf("connection from port %v does not exist", addr)
	}
	setStationSize := 3
	buf := make([]byte, setStationSize)
	if _, err := ms.ReadTimeout(conn, buf); err != nil {
		return ss, fmt.Errorf("failed to read message from conn: %w", err)
	}
	cmdType := uint8(buf[0])
	if cmdType != proto.SETSTATION_TYPE {
		return ss, fmt.Errorf("invalid type (expected %v, got %v)", proto.SETSTATION_TYPE, cmdType)
	}
	ss.StationNumber = util.Ntohs(buf[1:])
	return ss, nil
}

func (ms *Server) GetStationSong(stationId int) string {
	if stationId >= len(ms.Stations) {
		return ""
	}
	return ms.Stations[stationId].File.Name()
}
