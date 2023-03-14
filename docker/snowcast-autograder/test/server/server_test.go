package test

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"snowcast-autograder/config"
	"snowcast-autograder/mock"
	"snowcast-autograder/proto"
	"snowcast-autograder/student"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	SERVER_NAME       = "localhost"
	MIN_PORT          = 1024
	MAX_PORT          = 65535
	HIGH_COMMAND_TYPE = 255
	STARTUP_DELAY     = 100 * time.Millisecond
	N_CONNECTIONS     = 10
	STREAM_RATE       = 16000 // in Bytes/second
)

var (
	STATIONS_FILE = filepath.Join(config.JAIL_REPO, "stations")
	SHORT_FILE    = filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "short_file.mp3")
	mp3s          = []string{
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "Beethoven-SymphonyNo5.mp3"),
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "ManchurianCandidates-Breakin.mp3"),
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "VanillaIce-IceIceBaby.mp3"),
		SHORT_FILE,
	}
	// port = 10000
)

// attempts to generate a random port in [1024, 65535) 10 times; if none are free, randomly generate
// one and print an error.
func getPort() uint16 {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 10; i++ {
		port := rand.Intn(MAX_PORT-MIN_PORT) + MIN_PORT
		ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", port))

		if err == nil {
			ln.Close()
			return uint16(port)
		}
	}

	// if we can't generate it within 10 tries, just go for it and print an error if it spuriously
	// fails
	fmt.Println("Failed to get open port in 10 tries; will likely spuriously fail...")
	return uint16(rand.Intn(MAX_PORT-MIN_PORT) + MIN_PORT)
}

func contains[T comparable](s []T, e T) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

func unorderedEqual[T comparable](l1, l2 []T) bool {
	if len(l1) != len(l2) {
		return false
	}
	exists := map[T]struct{}{}
	for _, v := range l1 {
		exists[v] = struct{}{}
	}
	for _, v := range l2 {
		if _, ok := exists[v]; !ok {
			return false
		}
	}

	return true
}

type ServerTester struct {
	suite.Suite

	ss *student.Server
}

func TestServer(t *testing.T) {
	serverTester := new(ServerTester)
	suite.Run(t, serverTester)
}

func (st *ServerTester) SetupTest() {
	// kill all instances of the student server to free up ports
	student.KillProcesses()
}

func (st *ServerTester) TearDownTest() {
	// I don't actually think this is required, but whatever
	timer := time.AfterFunc(400*time.Millisecond, func() {
		st.ss.ForceDone(fmt.Errorf("sending error to unblock wait"))
	})
	if err := st.ss.Quit(); err != nil {
		st.ss.Wait()
	}
	timer.Stop()
	st.ss = nil
}

func (st *ServerTester) FlushOutErr() {
	stdout := st.ss.FlushStdout()
	if len(stdout) > 0 {
		fmt.Fprintf(os.Stderr, "===== STDOUT =====\n")
		for _, line := range stdout {
			fmt.Fprint(os.Stderr, line)
		}
		fmt.Println()
	}
	stderr := st.ss.FlushStderr()
	if len(stderr) > 0 {
		fmt.Fprintf(os.Stderr, "===== STDERR =====\n")
		for _, line := range stderr {
			fmt.Fprint(os.Stderr, line)
		}
		fmt.Println()
	}
}

// surely there must be a better way but whatever
func (st *ServerTester) Fatal(args ...any) {
	st.FlushOutErr()
	st.T().Fatal(args...)
}
func (st *ServerTester) Fatalf(format string, args ...any) {
	st.FlushOutErr()
	st.T().Fatalf(format, args...)
}

func (st *ServerTester) StartServer() {
	if err := st.ss.Start(); err != nil {
		st.Fatal(err)
	}
}

// Checks that the student server is still alive.
func (st *ServerTester) CheckAlive(reason string) {
	done, _ := st.ss.IsDone()
	if done {
		st.Fatalf("Student server should not have finished execution. %v", reason)
	}
}

// Checks that server has terminated without being killed (could have non-zero exit code though!)
func (st *ServerTester) CheckDoneAndNotKilled() {
	if st.ss.KilledOrNotDone() {
		st.Fatalf("Student control did not exit properly/was forcefully terminated after 250ms.")
	}
}

// Check that server exited cleanly (i.e. exit(0))
func (st *ServerTester) CheckExitedCleanly() {
	if done, err := st.ss.ExitedCleanly(); !done {
		if err != nil {
			st.Fatalf("Student server exited with non-zero error code: %v", err)
		} else {
			st.Fatalf("Student server has not terminated")
		}
	}
}

// Checks that the client received a Welcome with the correct number of stations.
func (st *ServerTester) CheckWelcome(mc *mock.Control) {
	if w, err := mc.GetWelcome(); err != nil {
		st.Fatalf("Failed to receive Welcome from student server: %v", err)
	} else {
		st.Equal(proto.Welcome{NumStations: uint16(len(st.ss.Files))}, w, "invalid number of stations")
	}
}

func (st *ServerTester) CheckAnnounce(mc *mock.Control, id uint16) {
	if a, err := mc.GetAnnounce(); err != nil {
		st.Fatalf("Failed to receive Announce from student server: %v", err)
	} else {
		// because people might not only send the song name in the announce, sanitize input and
		// simply check if it contains the song name or not
		songName := strings.ToLower(st.ss.Files[id])
		msg := strings.ToLower(a.SongName)
		st.True(strings.Contains(msg, songName), "Announce message must contain song name \"%v\" (got \"%v\")", songName, msg)
	}
}

// Checks whether the Station[id] [has/doesn't have] the address [laddr].
func (st *ServerTester) CheckStation(id uint16, has bool, laddr string) {
	// get server stations, and verify that client is part of the station with the specified ID
	if stations, err := st.ss.GetStations(STATIONS_FILE); err != nil {
		st.Fatalf("Failed to retrieve stations from student server: %v", err)
	} else {
		// fmt.Println("[CheckStation]", stations)
		if len(stations) != len(st.ss.Files) {
			st.Fatalf("Did not get enough stations from student server (expected %v, got %v)", len(st.ss.Files), len(stations))
		}
		// check that stations' conns list contains the listener addr
		if has {
			st.True(contains(stations[id].Conns, laddr), "Station %v's connections (%v) do not contain %v", id, stations[id].Conns, laddr)
		} else {
			st.False(contains(stations[id].Conns, laddr), "Station %v's connections (%v) contain %v", id, stations[id].Conns, laddr)
		}
	}
}

// Attempts to connect to a server on (SERVER_NAME, port, lport), and finish a handshake. I'm lazy x2
func (st *ServerTester) ConnectAndHandshake(port, lport uint16) *mock.Control {
	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}
	if err := mc.Hello(); err != nil {
		st.Fatalf("Failed to send Hello to student server: %v", err)
	}
	st.CheckWelcome(mc)

	return mc
}

// Sends a SetStation command and verifies server's Announce.
func (st *ServerTester) SetStationAndVerifyAnnounce(mc *mock.Control, id uint16) {
	if err := mc.SetStation(id); err != nil {
		st.Fatalf("Failed to send SetStation to student server: %v", err)
	}
	st.CheckAnnounce(mc, id)
}

func (st *ServerTester) ListenUDP(ip net.IP, port uint16) net.Conn {
	str := fmt.Sprintf("%s:%v", ip, port)
	// open UDP socket on (MockControl's IP):lport
	addr, err := net.ResolveUDPAddr("udp", str)
	if err != nil {
		st.Fatalf("Failed to parse address %v: %v", str, err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		st.Fatalf("Failed to open UDP socket on %v; this should not happen", addr)
	}
	return conn
}

func (st *ServerTester) VerifyStreamRate(conns []net.Conn) {
	nReads, totTimes := make([]int, len(conns)), make([]time.Duration, len(conns))

	// read until we've received 5*STREAM_RATE bytes
	threshold := 5 * STREAM_RATE
	buf := make([]byte, 4096)
	// yeah this is pretty scuffed but hey it kinda works
	done := make(chan int, len(conns))

	for i, c := range conns {
		go func(i int, c net.Conn) {
			for nReads[i] <= threshold {
				start := time.Now()
				n, _ := io.ReadAtLeast(c, buf, 1)
				nReads[i] += n
				totTimes[i] += time.Since(start)
			}
			done <- 1
		}(i, c)
	}
	// ensure that it doesn't take longer than 2*(threshold/STREAM_RATE) seconds, which is super generous
	timeout := 2 * time.Duration(threshold/STREAM_RATE) * time.Second
	timer := time.AfterFunc(timeout, func() {
		st.Fatalf(
			"Stream rate verification timed out after %v; should not take much more than %v",
			timeout,
			timeout/2,
		)
	})

	// wait for each to finish
	counter := 0
	for counter < len(conns) {
		counter += <-done
	}
	timer.Stop()

	// Give a range of 16KiB/s ± 2KiB, which should be pretty reasonable
	epsilon := float64(STREAM_RATE) / 8
	for i := range nReads {
		// compute rate in KiB/s
		rate := float64(nReads[i]) / (float64(totTimes[i]) / float64(time.Second))
		if math.Abs(rate-STREAM_RATE) > epsilon {
			st.Fatalf(
				"Stream rate (%vKiB/s) differs from expected (%vKiB/s) by more than the threshold (%vKiB/s)",
				rate/1000,
				STREAM_RATE/1000,
				epsilon/1000,
			)
		}
	}
}

/* ========== BEGIN TESTS ========== */

func (st *ServerTester) TestNoStationsFails() {
	// generate port for student server
	port := getPort()
	// create student server, and assign suite's StudentServer to the newly created one
	ss, err := student.NewServer(port, nil)
	if err != nil {
		st.Fatalf("Failed to initialized student server: %v", err)
	}
	st.ss = ss
	// start student server
	go st.StartServer()
	// give server time to start up
	time.Sleep(STARTUP_DELAY)

	// Server should have shut down
	st.ss.Wait()
	// check that it exited successfully without being killed
	st.CheckDoneAndNotKilled()
}

func (st *ServerTester) TestAcceptsClientConnection() {
	port, lport := getPort(), getPort()
	// use only one station
	ss, err := student.NewServer(port, []string{mp3s[0]})
	if err != nil {
		st.Fatalf(
			"Failed to initialize student server: %v. NOTE: this may spuriously fail on the grading server; if so, please re-run the test.",
			err,
		)
	}
	st.ss = ss
	// start student server
	go st.StartServer()
	// give server time to start up
	time.Sleep(STARTUP_DELAY)

	// Check that server is still alive
	st.CheckAlive(
		"Check that your executable is named properly (snowcast_server) and correctly accepts command line arguments. NOTE: this may also spuriously fail on the grading server; if so, please re-run the test.",
	)

	// attempt to connect to server
	if _, err = mock.NewControl(SERVER_NAME, port, lport); err != nil {
		st.Fatalf(
			"Failed to connect to student server: %v. NOTE: this may spuriously fail on the grading server; if so, please re-run the test.",
			err,
		)
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestTimesOutConnectionWithNoHello() {
	port, lport := getPort(), getPort()
	// use only one station
	ss, err := student.NewServer(port, []string{mp3s[0]})
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	// start student server
	go st.StartServer()
	// give server time to start up
	time.Sleep(STARTUP_DELAY)

	// attempt to connect to server
	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}

	// sleep for 250ms, then check that client connection has been closed
	time.Sleep(250 * time.Millisecond)
	if !mc.ConnClosed() {
		st.Fatalf("Client connection to student server should have been closed")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestCompletesHandshake() {
	// startup...
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, []string{SHORT_FILE})
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// check that server is still running
	st.CheckAlive(
		"Check that your executable is named properly (snowcast_server) and correctly accepts command line arguments!",
	)

	// verify handshake works
	st.ConnectAndHandshake(port, lport)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestMultipleHellosFails() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// initiate handshake
	mc := st.ConnectAndHandshake(port, lport)
	// Send another Hello
	if err := mc.Hello(); err != nil {
		st.Fatalf("Failed to send Hello to student server: %v", err)
	}

	// sleep a little bit to give time for a response
	time.Sleep(10 * time.Millisecond)
	// Check that connection is now closed
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// Check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestPartialHelloTimeoutFails() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// attempt to connect to server
	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}
	// create hello buffer
	h := proto.Hello{UDPPort: lport}
	buf := h.Serialize()
	// send server only part of the Hello
	if _, err := mc.WriteTimeout(buf[:1]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}

	// wait 250ms, without finishing send
	time.Sleep(250 * time.Millisecond)

	// Connection should now be closed
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSplitHelloSucceeds1() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// attempt to connect to server
	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}
	// create hello buffer
	h := proto.Hello{UDPPort: lport}
	buf := h.Serialize()
	// send server only part of the Hello
	if _, err := mc.WriteTimeout(buf[:1]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}

	// wait 75ms, without finishing send
	time.Sleep(75 * time.Millisecond)

	// send the rest of the buffer
	if _, err := mc.WriteTimeout(buf[1:]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}
	// Server should have sent a Welcome
	st.CheckWelcome(mc)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSplitHelloSucceeds2() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// attempt to connect to server
	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}
	// create hello buffer
	h := proto.Hello{UDPPort: lport}
	buf := h.Serialize()
	// send Hello byte by byte, sleeping 25ms in between
	for _, b := range buf {
		if _, err := mc.WriteTimeout([]byte{b}); err != nil {
			st.Fatalf("Failed to send buffer to student control: %v", err)
		}
		// wait 25ms, without finishing send
		time.Sleep(25 * time.Millisecond)
	}

	// Server should have sent a Welcome
	st.CheckWelcome(mc)

	// Server should still be alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSetStationBeforeHelloFails() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc, err := mock.NewControl(SERVER_NAME, port, lport)
	if err != nil {
		st.Fatalf("Failed to connect to student server: %v", err)
	}
	// send SetStation
	if err := mc.SetStation(0); err != nil {
		st.Fatalf("Failed to send SetStation to student server: %v", err)
	}

	// sleep a little bit to give time for a response
	time.Sleep(10 * time.Millisecond)
	// Check that connection is now closed
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSetStationSucceeds() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// Initialize handshake
	mc := st.ConnectAndHandshake(port, lport)

	// Send server a SetStation, and verify announce response
	var id uint16 = 2
	st.SetStationAndVerifyAnnounce(mc, id)

	// check that the server's station 2 has the connection in list
	laddr := fmt.Sprintf("%s:%v", mc.ConnIP(), lport)
	st.CheckStation(id, true, laddr)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestPartialSetStationTimeoutFails() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// create SetStation buffer
	var id uint16 = 2
	s := proto.SetStation{StationNumber: id}
	buf := s.Serialize()
	// send server only part of the SetStation
	if _, err := mc.WriteTimeout(buf[:2]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}

	// wait 250ms, without finishing send
	time.Sleep(250 * time.Millisecond)

	// Connection should now be closed
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSplitSetStationSucceeds1() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// create SetStation buffer
	var id uint16 = 2
	s := proto.SetStation{StationNumber: id}
	buf := s.Serialize()
	// send server only part of the SetStation
	if _, err := mc.WriteTimeout(buf[:2]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}

	// wait 75ms, without finishing send
	time.Sleep(75 * time.Millisecond)

	// send the rest of the buffer
	if _, err := mc.WriteTimeout(buf[2:]); err != nil {
		st.Fatalf("Failed to send buffer to student server: %v", err)
	}

	// Server should have sent an Announce
	st.CheckAnnounce(mc, id)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSplitSetStationSucceeds2() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// create SetStation buffer
	var id uint16 = 2
	s := proto.SetStation{StationNumber: id}
	buf := s.Serialize()
	// send byte by byte
	for _, b := range buf {
		if _, err := mc.WriteTimeout([]byte{b}); err != nil {
			st.Fatalf("Failed to send buffer to student server: %v", err)
		}
		// wait 25ms, without finishing send
		time.Sleep(25 * time.Millisecond)
	}

	// Server should have sent an Announce
	st.CheckAnnounce(mc, id)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSupportsMultipleStations() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// check that station list has every station, in order
	if stations, err := ss.GetStations(STATIONS_FILE); err != nil {
		st.Fatalf("Failed to get stations from student server: %v", err)
	} else {
		for i := range stations {
			st.Equal(ss.Files[i], stations[i].SongName, "Song names do not match")
		}
	}

	// set station to each one and verify veracity of the Announce, then check that the server's
	// station has the connection in the list and the previous one doesn't have it
	laddr := fmt.Sprintf("%s:%v", mc.ConnIP(), lport)
	for id := range mp3s {
		st.SetStationAndVerifyAnnounce(mc, uint16(id))
		st.CheckStation(uint16(id), true, laddr)
		if id > 0 {
			st.CheckStation(uint16(id-1), false, laddr)
		}
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestStreamsWithoutReadingEntireFile() {
	port, lport := getPort(), getPort()
	files := []string{"/dev/urandom", "/dev/zero"}
	files = append(append(files, files...), files...)
	ss, err := student.NewServer(port, files)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// set station to each one and verify veracity of the Announce, then check that the server's
	// station has the connection in the list and the previous one doesn't have it
	laddr := fmt.Sprintf("%s:%v", mc.ConnIP(), lport)
	for id := range files {
		st.SetStationAndVerifyAnnounce(mc, uint16(id))
		st.CheckStation(uint16(id), true, laddr)
		if id > 0 {
			st.CheckStation(uint16(id-1), false, laddr)
		}
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestInvalidStationFails() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// Send server a bogus SetStation
	if err := mc.SetStation(65535); err != nil {
		st.Fatalf("Failed to send SetStation to student server: %v", err)
	}

	// sleep a little bit to give time for a response
	time.Sleep(10 * time.Millisecond)
	// Check that connection is now closed
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestAcceptsMultipleConnections() {
	port := getPort()
	files := append(mp3s, mp3s...)
	ss, err := student.NewServer(port, files)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// connect N_CONNECTIONS clients
	lports := make([]uint16, N_CONNECTIONS)
	mcs := make([]*mock.Control, N_CONNECTIONS)
	for i := range lports {
		lports[i] = getPort()
		mcs[i] = st.ConnectAndHandshake(port, lports[i])
	}

	finalStations := make([]uint16, N_CONNECTIONS)
	// for each one, randomly pick a station to which to connect
	for i := range mcs {
		id := uint16(rand.Intn(len(files)))
		st.SetStationAndVerifyAnnounce(mcs[i], id)
		finalStations[i] = id
	}

	// aggregate desired connections for each file
	fileConns := map[uint16][]string{}
	for i := 0; i < len(files); i++ {
		fileConns[uint16(i)] = make([]string, 0)
	}
	for i := range finalStations {
		laddr := fmt.Sprintf("%s:%v", mcs[i].ConnIP(), lports[i])
		fileConns[finalStations[i]] = append(fileConns[finalStations[i]], laddr)
	}

	// get stations, and check that each station has the same list of connections
	stations, err := ss.GetStations(STATIONS_FILE)
	if err != nil {
		st.Fatalf("Failed to retrieve stations from student server: %v", err)
	} else if len(stations) != len(files) {
		st.Fatalf("did not get the right number of stations (expected %v, got %v)", len(files), len(stations))
	}
	for _, station := range stations {
		st.True(
			unorderedEqual(station.Conns, fileConns[uint16(station.ID)]),
			"Connections do not match (got %v, expected %v)",
			station.Conns,
			fileConns[uint16(station.ID)],
		)
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestStreamRateSingle() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// connect to server and set station to 0
	mc := st.ConnectAndHandshake(port, lport)
	st.SetStationAndVerifyAnnounce(mc, 0)
	// open UDP socket on (MockControl's IP):lport
	conn := st.ListenUDP(mc.ConnIP(), lport)

	// verify stream rate
	st.VerifyStreamRate([]net.Conn{conn})
}

func (st *ServerTester) TestStreamRateMultipleOneStation() {
	port := getPort()
	files := []string{mp3s[0]}
	ss, err := student.NewServer(port, files)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// connect N_CONNECTIONS clients
	lports := make([]uint16, N_CONNECTIONS)
	for i := range lports {
		lports[i] = getPort()
	}
	// Set their station to 0, and create UDP listener
	mcs := make([]*mock.Control, N_CONNECTIONS)
	conns := make([]net.Conn, N_CONNECTIONS)
	for i := range mcs {
		mcs[i] = st.ConnectAndHandshake(port, lports[i])
		st.SetStationAndVerifyAnnounce(mcs[i], 0)
		conns[i] = st.ListenUDP(mcs[i].ConnIP(), lports[i])
	}

	// verify stream rate
	st.VerifyStreamRate(conns)
}

func (st *ServerTester) TestStreamRateMultipleStations() {
	port := getPort()
	files := make([]string, N_CONNECTIONS)
	for i := range files {
		files[i] = mp3s[0]
	}
	ss, err := student.NewServer(port, files)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// connect N_CONNECTIONS clients
	lports := make([]uint16, N_CONNECTIONS)
	for i := range lports {
		lports[i] = getPort()
	}
	// Set to their corresponding station, and create UDP listener
	mcs := make([]*mock.Control, N_CONNECTIONS)
	conns := make([]net.Conn, N_CONNECTIONS)
	for i := range mcs {
		mcs[i] = st.ConnectAndHandshake(port, lports[i])
		st.SetStationAndVerifyAnnounce(mcs[i], uint16(i))
		conns[i] = st.ListenUDP(mcs[i].ConnIP(), lports[i])
	}

	// verify stream rate
	st.VerifyStreamRate(conns)
}

func (st *ServerTester) TestStreamsWithNoListeners() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, []string{SHORT_FILE})
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	// connect to server, then sleep for a second
	mc := st.ConnectAndHandshake(port, lport)
	time.Sleep(time.Second)

	// set station to 0
	st.SetStationAndVerifyAnnounce(mc, 0)
	// short_file.mp3 is of size ~20KiB, so it should take a little more than a second to
	// receive an announce; assuming 16KiB/s, give the server up to 0.5s to generate an Announce
	mc.SetTimeout(500 * time.Millisecond)
	st.CheckAnnounce(mc, 0)
}

func (st *ServerTester) TestClosesConnectionOnInvalidCommandType() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)
	mc := st.ConnectAndHandshake(port, lport)

	// Send server an invalid command type
	if err := mc.InvalidCommand(HIGH_COMMAND_TYPE); err != nil {
		st.Fatalf("Failed to send an invalid command to student server: %v", err)
	}

	// Wait a little, then check that connection is now closed (TODO: I don't think we can check for
	// an InvalidCommand reply, since they should be closing the socket, which would yield
	// EOF/ECONNRESET.)
	time.Sleep(10 * time.Millisecond)
	if !mc.ConnClosed() {
		st.Fatalf("Student server should have closed the connection to the control client")
	}

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestSurvivesClientDisconnect() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// connect to station 2
	var id uint16 = 2
	st.SetStationAndVerifyAnnounce(mc, id)

	// check that connection is part of the station
	laddr := fmt.Sprintf("%s:%v", mc.ConnIP(), lport)
	st.CheckStation(id, true, laddr)

	// disconnect
	mc.Quit()

	// wait a bit, then check that connection is no longer part of the station
	time.Sleep(10 * time.Millisecond)
	st.CheckStation(id, false, laddr)

	// check that server is still alive
	st.CheckAlive("")
}

func (st *ServerTester) TestQuitsCleanly() {
	port, lport := getPort(), getPort()
	ss, err := student.NewServer(port, mp3s)
	if err != nil {
		st.Fatalf("Failed to initialize student server: %v", err)
	}
	st.ss = ss
	go st.StartServer()
	time.Sleep(STARTUP_DELAY)

	mc := st.ConnectAndHandshake(port, lport)

	// quit from server, and check exit
	if err := ss.Quit(); err != nil {
		st.Fatalf("Failed to quit student server: %v", err)
	}
	st.CheckExitedCleanly()

	// give some time, then check if connection closed
	time.Sleep(10 * time.Millisecond)
	if !mc.ConnClosed() {
		st.Fatalf("Client connection to student server should have been closed")
	}
}
