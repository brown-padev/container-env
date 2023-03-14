package test

import (
	"fmt"
	"math/rand"
	"net"
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
	SERVER_NAME     = "localhost"
	MIN_PORT        = 1024
	MAX_PORT        = 65535
	HIGH_REPLY_TYPE = 255
	TIMEOUT         = 100 * time.Millisecond
	STARTUP_DELAY   = 25 * time.Millisecond
)

var (
	mp3s = []string{
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "Beethoven-SymphonyNo5.mp3"),
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "ManchurianCandidates-Breakin.mp3"),
		filepath.Join(config.AUTOGRADER_REPO, config.MP3_REPO, "VanillaIce-IceIceBaby.mp3"),
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

type ControlTester struct {
	suite.Suite

	port  uint16
	lport uint16
	ms    *mock.Server
	sc    *student.Control
}

func TestControl(t *testing.T) {
	controlTester := new(ControlTester)
	suite.Run(t, controlTester)
}

func (ct *ControlTester) SetupTest() {
	ct.port, ct.lport = getPort(), getPort()
	ms, err := mock.NewServer(ct.port, mp3s)
	if err != nil {
		ct.Fatalf(
			"Failed to initialize server: %v. This is a grading server issue (some tests may spuriously fail, for reasons you'll learn in IP/TCP); please re-run the test!",
			err,
		)
	}
	ct.ms = ms
	sc, err := student.NewControl(SERVER_NAME, ct.port, ct.lport)
	if err != nil {
		ct.Fatalf("Failed to initialize student control: %v", err)
	}
	ct.sc = sc
}

func (ct *ControlTester) TearDownTest() {
	ct.port, ct.lport = 0, 0

	ct.ms.Quit()
	ct.ms, ct.sc = nil, nil
}

// surely there must be a better way but whatever
func (ct *ControlTester) Fatal(args ...any) {
	ct.T().Fatal(args...)
}
func (ct *ControlTester) Fatalf(format string, args ...any) {
	ct.T().Fatalf(format, args...)
}

// Starts student control. Blocks; run in a goroutine
func (ct *ControlTester) StartControl() {
	if err := ct.sc.Start(); err != nil {
		ct.Fatal(err)
	}
}

// Checks that the server received a Hello with the correct port.
func (ct *ControlTester) CheckHello(addr net.Addr) {
	h, err := ct.ms.GetHello(addr)
	if err != nil {
		ct.Fatalf("Failed to receive Hello from student control: %v", err)
	}
	ct.Equal(proto.Hello{UDPPort: ct.lport}, h, "invalid listener port")
}

// Checks that the client received a Welcome with the correct number of stations.
func (ct *ControlTester) CheckWelcome() {
	numStations := len(ct.ms.Stations)
	if !ct.sc.StdoutContains(fmt.Sprint(numStations), "station") {
		ct.Fatalf(
			"Student control did not read/print the correct number of stations (expected %v). Make sure that your control prints **to stdout** the message \"Welcome to Snowcast! The server has `N` stations\", where `N` is the number of stations received in the Welcome message (make sure that you flush stdout as well).",
			numStations,
		)
	}
}

// Accepts a connection, then executes a handshake. I'm lazy ok
func (ct *ControlTester) AcceptAndHandshake() net.Addr {
	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)
	if err := ct.ms.Welcome(addr); err != nil {
		ct.Fatalf("failed to send welcome: %v", err)
	}
	// sleep to give enough time to print
	time.Sleep(10 * time.Millisecond)
	// output should contain the number of stations in a line
	ct.CheckWelcome()

	return addr
}

// Checks that the client is still alive.
func (ct *ControlTester) CheckAlive(reason string) {
	done, _ := ct.sc.IsDone()
	if done {
		ct.Fatalf("Student control should not have finished execution. %v", reason)
	}
}

// Check that program exited cleanly (i.e. exit(0))
func (ct *ControlTester) CheckExitedCleanly() {
	if done, err := ct.sc.ExitedCleanly(); !done {
		if err != nil {
			ct.Fatalf("Student control exited with non-zero error code: %v", err)
		} else {
			ct.Fatalf("Student control has not terminated")
		}
	}
}

// Checks that the program has exited, but was not killed (non-zero exit codes allowed!).
func (ct *ControlTester) CheckDoneAndNotKilled() {
	if ct.sc.KilledOrNotDone() {
		ct.Fatalf("Student control did not exit properly/was forcefully terminated after 500ms.")
	}
}

func (ct *ControlTester) TestConnectsToServer() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// check that client is alive
	ct.CheckAlive(
		"Check that your executable is named properly (snowcast_control) and correctly accepts command line arguments!",
	)

	// attempt to accept a connection, timing out after a second
	addr, err := ct.ms.Accept(time.Second)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}

	// attempts to read a Hello message from the client; timeouts after 250ms (see ms.timeout)
	ct.CheckHello(addr)
}

func (ct *ControlTester) TestCompletesHandshake() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// check that client is alive
	ct.CheckAlive(
		"Check that your executable is named properly (snowcast_control) and correctly accepts command line arguments!",
	)

	// check handshake
	ct.AcceptAndHandshake()
}

func (ct *ControlTester) TestMultipleWelcomesFails() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)

	// Send two welcomes
	for range [2]int{} {
		if err := ct.ms.Welcome(addr); err != nil {
			ct.Fatalf("Failed to send Welcome to student control: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// should give successful quit
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestNoWelcomeFails() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)

	// wait for 250ms without sending a welcome
	time.Sleep(250 * time.Millisecond)

	// the client control should have quit by now
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestPartialWelcomeTimeoutFails() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)

	// Create welcome buffer
	w := proto.Welcome{NumStations: uint16(len(ct.ms.Stations))}
	buf := w.Serialize()

	// Send only half of the welcome
	if _, err := ct.ms.WriteAddr(addr, buf[:1]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}
	// wait 250ms, without finishing send
	time.Sleep(250 * time.Millisecond)

	// client control should have quit by now
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestSplitWelcomeSucceeds1() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)

	// Create welcome buffer
	w := proto.Welcome{NumStations: uint16(len(ct.ms.Stations))}
	buf := w.Serialize()

	// Send only part of the welcome
	if _, err := ct.ms.WriteAddr(addr, buf[:1]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}
	// wait 75ms, without finishing send
	time.Sleep(75 * time.Millisecond)
	// send the rest of the buffer
	if _, err := ct.ms.WriteAddr(addr, buf[1:]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}

	// sleep to give enough time to print
	time.Sleep(50 * time.Millisecond)
	// check that welcome has been sent successfully
	ct.CheckWelcome()

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestSplitWelcomeSucceeds2() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	// give the control client 250ms to connect
	addr, err := ct.ms.Accept(250 * time.Millisecond)
	if err != nil {
		ct.Fatalf("Failed to receive student control connection: %v", err)
	}
	ct.CheckHello(addr)

	// Create welcome buffer
	w := proto.Welcome{NumStations: uint16(len(ct.ms.Stations))}
	buf := w.Serialize()

	// send welcome byte by byte, sleeping 25ms in between
	for _, b := range buf {
		if _, err := ct.ms.WriteAddr(addr, []byte{b}); err != nil {
			ct.Fatalf("Failed to send buffer to student control: %v", err)
		}
		// wait 25ms, without finishing send
		time.Sleep(25 * time.Millisecond)
	}
	// check that welcome has been sent successfully
	ct.CheckWelcome()

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestSetStationFormat() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	if err := ct.sc.SetStation(1); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}

	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: 1}, ss, "invalid set station value")
	}

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestPrintsAnnounce1() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}

	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "Invalid SetStation value")
	}

	songName := strings.ToLower(ct.ms.GetStationSong(int(STATION_ID)))
	// send announce back
	if err := ct.ms.Announce(addr, songName); err != nil {
		ct.Fatalf("failed to send Announce to student control: %v", err)
	}

	// wait for stdout to print, then check that announcement is made on client side
	time.Sleep(10 * time.Millisecond)
	if !ct.sc.StdoutContains("announce", songName) {
		ct.Fatalf("Student control did not print on announce")
	}

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestPrintsAnnounce2() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}
	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "invalid set station value")
	}

	songName := strings.ToLower(ct.ms.GetStationSong(int(STATION_ID)))
	// do this 10 times
	for range [10]int{} {
		// send announce back
		if err := ct.ms.Announce(addr, songName); err != nil {
			ct.Fatalf("failed to send Announce to student control: %v", err)
		}
		// wait for stdout to print, then check that announcement is made on client side
		time.Sleep(10 * time.Millisecond)
		if !ct.sc.StdoutContains("announce", songName) {
			ct.Fatalf("Student control did not print on announce")
		}
	}

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestPartialAnnounceTimeoutFails() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()
	// Send SetStation from control
	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}
	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "invalid set station value")
	}
	songName := strings.ToLower(ct.ms.GetStationSong(int(STATION_ID)))
	// Create Announce buffer
	a := proto.Announce{SongName: songName}
	buf := a.Serialize()

	// Send only part of the announce
	if _, err := ct.ms.WriteAddr(addr, buf[:2]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}
	// wait 250ms, without finishing send
	time.Sleep(250 * time.Millisecond)

	// client control should have quit by now
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestSplitAnnounceSucceeds1() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()
	// Send SetStation from control
	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}
	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "invalid set station value")
	}
	songName := strings.ToLower(ct.ms.GetStationSong(int(STATION_ID)))
	// Create Announce buffer
	a := proto.Announce{SongName: songName}
	buf := a.Serialize()

	// Send only part of the announce
	if _, err := ct.ms.WriteAddr(addr, buf[:2]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}
	// wait 75ms, without finishing send
	time.Sleep(75 * time.Millisecond)
	// send the rest of the buffer
	if _, err := ct.ms.WriteAddr(addr, buf[2:]); err != nil {
		ct.Fatalf("Failed to send buffer to student control: %v", err)
	}

	// check that Announce has been sent successfully
	time.Sleep(10 * time.Millisecond)
	if !ct.sc.StdoutContains("announce", songName) {
		ct.Fatalf("Student control did not print on announce")
	}

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestSplitAnnounceSucceeds2() {
	// start student control
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()
	// Send SetStation from control
	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}
	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "invalid set station value")
	}
	songName := strings.ToLower(ct.ms.GetStationSong(int(STATION_ID)))
	// Create Announce buffer
	a := proto.Announce{SongName: songName}
	buf := a.Serialize()

	delay := (TIMEOUT / 2) / time.Duration(len(buf))
	// send Announce byte by byte, sleeping 10ms in between
	for _, b := range buf {
		if _, err := ct.ms.WriteAddr(addr, []byte{b}); err != nil {
			ct.Fatalf("Failed to send buffer to student control: %v", err)
		}
		time.Sleep(delay)
	}

	// check that Announce has been sent successfully
	time.Sleep(10 * time.Millisecond)
	if !ct.sc.StdoutContains("announce", songName) {
		ct.Fatalf("Student control did not print on announce")
	}

	// ensure that program is still alive
	ct.CheckAlive("")
}

func (ct *ControlTester) TestAnnounceBeforeSetStationFails() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	songName := ct.ms.GetStationSong(1)
	// send announce without set station
	if err := ct.ms.Announce(addr, songName); err != nil {
		ct.Fatalf("failed to send Announce to student control: %v", err)
	}

	// program should have shut down
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestSetStationInvalidCommandResponseFails() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	var STATION_ID uint16 = 1
	if err := ct.sc.SetStation(STATION_ID); err != nil {
		ct.Fatalf("failed to send set station from student control: %v", err)
	}

	if ss, err := ct.ms.GetSetStation(addr); err != nil {
		ct.Fatalf("failed to receive set station from student control: %v", err)
	} else {
		ct.Equal(proto.SetStation{StationNumber: STATION_ID}, ss, "invalid set station value")
	}

	// send welcome back, instead of announce
	if err := ct.ms.InvalidCommand(addr, "invalid command test"); err != nil {
		ct.Fatalf("failed to send InvalidCommand to student control: %v", err)
	}

	// program should have shut down
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestExitsOnInvalidCommand() {
	// startup
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	time.Sleep(10 * time.Millisecond)
	// send InvalidCommand
	if err := ct.ms.InvalidCommand(addr, "invalid command test"); err != nil {
		ct.Fatalf("failed to send invalid command: %v", err)
	}

	// wait for program to terminate
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestExitsOnInvalidReplyType() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	addr := ct.AcceptAndHandshake()

	// send a reply with an invalid type
	if err := ct.ms.InvalidReply(addr, HIGH_REPLY_TYPE); err != nil {
		ct.Fatalf("failed to send invalid reply: %v", err)
	}

	// wait for program to terminate
	ct.sc.Wait()
	// check that it exited successfully without being killed
	ct.CheckDoneAndNotKilled()
}

func (ct *ControlTester) TestQuitsCleanly() {
	go ct.StartControl()
	time.Sleep(STARTUP_DELAY)

	ct.AcceptAndHandshake()

	// quit the control client
	if err := ct.sc.Quit(); err != nil {
		ct.Fatalf("Failed to quit student control: %v", err)
	}

	// Check that process exited cleanly
	ct.CheckExitedCleanly()
}
