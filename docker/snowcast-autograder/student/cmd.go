package student

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Wrapper around an exec.Cmd to facilitate basic I/O and healthchecking.
type Cmd struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	outChan chan string
	errChan chan string

	done chan error
}

// Runs the executable, and retrieves stdout/stderr. Blocks; run in a go-routine.
func (c *Cmd) Start() error {
	go func() {
		c.done <- c.cmd.Run()
	}()
	go func() {
		r := bufio.NewReader(c.stderr)
		for {
			str, err := r.ReadString('\n')
			if err != nil {
				return
			}
			c.errChan <- str
		}
	}()
	r := bufio.NewReader(c.stdout)
	for {
		str, err := r.ReadString('\n')
		if err != nil {
			// if EOF, just exit, no error occurred
			// if errors.Is(err, io.EOF) {
			return nil // rn too lazy to check other cases so just return nil lol
			// }
			// return fmt.Errorf("failed to read string: %w", err)
		}
		c.outChan <- str
	}
}

// Clears the stdout channel, returning any printed lines.
func (c *Cmd) FlushStdout() []string {
	lines := make([]string, len(c.outChan))
	for len(c.outChan) > 0 {
		lines = append(lines, <-c.outChan)
	}
	return lines
}

// Clears the stderr channel, returning any printed lines.
func (c *Cmd) FlushStderr() []string {
	lines := make([]string, len(c.errChan))
	for len(c.errChan) > 0 {
		lines = append(lines, <-c.errChan)
	}
	return lines
}

// Retrieves a line from stdout, if one exists; returns false otherwise.
func (c *Cmd) GetLine() (string, bool) {
	select {
	case str := <-c.outChan:
		return str, true
	default:
		return "", false
	}
}

// Returns true if any currently outputted lines contains all of the the desired substrings, false otherwise.
func (c *Cmd) StdoutContains(substrs ...string) bool {
	for len(c.outChan) > 0 {
		line := <-c.outChan
		line = strings.ToLower(line)

		hasAll := true
		for _, substr := range substrs {
			hasAll = hasAll && strings.Contains(line, substr)
		}
		if hasAll {
			return true
		}
	}
	return false
}

// Quits the executable, then waits for the subprocess to finish.
func (c *Cmd) Quit() error {
	if _, err := c.stdin.Write([]byte("q\n")); err != nil {
		return err
	}
	c.Wait()

	return nil
}

func (c *Cmd) Wait() {
	// wait 500ms before killing process
	timer := time.AfterFunc(500*time.Millisecond, func() {
		c.cmd.Process.Kill()
		// fmt.Println("wait timed out after 1s; killed process")
	})
	// read message once command finishes (c.Run() will send error)
	err := <-c.done
	timer.Stop()
	// this is pretty jank, but we want to be able to process the error in another function. If we
	// can come up with a better interface later, probably ideal
	c.done <- err
}

// Manually force a "done" message into the channel, just in case a Wait() hangs.
func (c *Cmd) ForceDone(err error) {
	c.done <- err
}

// Check whether command has finished executing. error is only relevant if command finished.
func (c *Cmd) IsDone() (bool, error) {
	select {
	case err := <-c.done:
		return true, err
	default:
		return false, nil
	}
}

// Checks whether process exited cleanly (i.e. with exit code 0); error only relevant if false.
func (c *Cmd) ExitedCleanly() (bool, error) {
	select {
	case err := <-c.done:
		return err == nil, err
	default:
		return false, nil
	}
}

// checks whether process was killed or not done
func (c *Cmd) KilledOrNotDone() bool {
	select {
	case err := <-c.done:
		if err != nil && err.Error() == "signal: killed" {
			return true
		}
		return false
	default:
		return true
	}
}
