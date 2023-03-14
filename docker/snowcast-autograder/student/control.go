package student

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"snowcast-autograder/config"
)

var (
	STUDENT_CONTROL = filepath.Join(config.JAIL_REPO, "snowcast_control")
)

type Control struct {
	Cmd
	ServerName   string
	ServerPort   uint16
	ListenerPort uint16
}

// Creates a new student control struct wrapping a student_control subprocess.
func NewControl(serverName string, serverPort, listenerPort uint16) (*Control, error) {
	ctrlCmd := exec.Command(
		STUDENT_CONTROL,
		serverName,
		fmt.Sprint(serverPort),
		fmt.Sprint(listenerPort),
	)
	inPipe, err := ctrlCmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	outPipe, err := ctrlCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errPipe, err := ctrlCmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	sc := Control{
		Cmd: Cmd{
			cmd:     ctrlCmd,
			stdin:   inPipe,
			stdout:  outPipe,
			stderr:  errPipe,
			outChan: make(chan string, 1024),
			errChan: make(chan string, 1024),
			done:    make(chan error, 1),
		},
		ServerName:   serverName,
		ServerPort:   serverPort,
		ListenerPort: listenerPort,
	}

	return &sc, nil
}

// Sets the control client's listening station to the station with the desired ID.
func (sc *Control) SetStation(stationId uint16) error {
	if _, err := sc.stdin.Write([]byte(fmt.Sprintf("%v\n", stationId))); err != nil {
		return err
	}

	return nil
}
