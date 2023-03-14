package student

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"snowcast-autograder/config"
	"strconv"
	"strings"
	"time"
)

var (
	STUDENT_SERVER = filepath.Join(config.JAIL_REPO, "snowcast_server")
)

type Station struct {
	ID       uint
	SongName string
	Conns    []string
}

type Server struct {
	Cmd            // embedded struct to represent Cmd wrapper
	Port  uint16   // server port
	Files []string // server station files
}

func NewServer(port uint16, files []string) (*Server, error) {
	serverCmd := exec.Command(STUDENT_SERVER, append([]string{fmt.Sprint(port)}, files...)...)
	inPipe, err := serverCmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	outPipe, err := serverCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errPipe, err := serverCmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	ss := Server{
		Cmd: Cmd{
			cmd:     serverCmd,
			stdin:   inPipe,
			stdout:  outPipe,
			stderr:  errPipe,
			outChan: make(chan string, 1024),
			errChan: make(chan string, 1024),
			done:    make(chan error, 1),
		},
		Port:  port,
		Files: files,
	}
	// fmt.Println(serverCmd.Args)

	return &ss, nil
}

func KillProcesses() {
	args := []string{"-9", "-f", STUDENT_SERVER}
	cmd := exec.Command("pkill", args...)
	cmd.Run()
	// if err != nil {
	// fmt.Println("[KillProcesses]:", err)
	// }
}

// Gets the stations, plus a list of all connected clients.
func (ss *Server) GetStations(dst string) ([]Station, error) {
	// remove, just in case it already exists
	os.Remove(dst)
	cmd := fmt.Sprintf("p %s\n", dst)
	if _, err := ss.stdin.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("failed to execute cmd %s: %w", cmd, err)
	}

	time.Sleep(100 * time.Millisecond)
	f, err := os.Open(dst)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create/open stations file. This is likely because your server has not yet implemented printing stations to a file (`p <file>`)",
		)
	}
	defer f.Close()

	stations := []Station{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.Split(s.Text(), ",")
		id, err := strconv.Atoi(line[0])
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID: %w", err)
		}
		songName := line[1]

		conns := make([]string, len(line[2:]))
		copy(conns, line[2:])
		stations = append(stations, Station{ID: uint(id), SongName: songName, Conns: conns})
	}

	return stations, nil
}
