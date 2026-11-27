package luabridge

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
)

// RedirectStdio redirects process os.Stdout, os.Stderr, and standard log output into this Console
// (optionally teeing back to the original terminal descriptors when TeeStdio is true).
// Call RestoreStdio() on shutdown to drain pipes cleanly and restore descriptors.
func (c *Console) RedirectStdio() error {
	if c == nil {
		return nil
	}
	c.stdioMu.Lock()
	defer c.stdioMu.Unlock()
	if c.stdioActive {
		return nil
	}

	rOut, wOut, err := os.Pipe()
	if err != nil {
		return err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		_ = rOut.Close()
		_ = wOut.Close()
		return err
	}

	c.origStdout = os.Stdout
	c.origStderr = os.Stderr
	c.origLogOut = log.Writer()
	c.outPipeW = wOut
	c.errPipeW = wErr

	os.Stdout = wOut
	os.Stderr = wErr
	log.SetOutput(wErr)
	c.stdioActive = true

	c.stdioWG.Add(2)
	go c.pumpPipe(rOut, c.origStdout.(*os.File), LineOutput)
	go c.pumpPipe(rErr, c.origStderr.(*os.File), LineError)
	return nil
}

// RestoreStdio closes pipe writers, drains reader goroutines, and restores original os.Stdout, os.Stderr, and log output.
func (c *Console) RestoreStdio() {
	if c == nil {
		return
	}
	c.stdioMu.Lock()
	if !c.stdioActive {
		c.stdioMu.Unlock()
		return
	}
	if origOut, ok := c.origStdout.(*os.File); ok && origOut != nil {
		os.Stdout = origOut
	}
	if origErr, ok := c.origStderr.(*os.File); ok && origErr != nil {
		os.Stderr = origErr
	}
	if c.origLogOut != nil {
		log.SetOutput(c.origLogOut)
	}
	if wOut, ok := c.outPipeW.(*os.File); ok && wOut != nil {
		_ = wOut.Close()
	}
	if wErr, ok := c.errPipeW.(*os.File); ok && wErr != nil {
		_ = wErr.Close()
	}
	c.outPipeW = nil
	c.errPipeW = nil
	c.stdioActive = false
	c.stdioMu.Unlock()

	c.stdioWG.Wait()
}

func (c *Console) pumpPipe(reader *os.File, orig *os.File, lineType LineType) {
	defer c.stdioWG.Done()
	defer reader.Close()

	br := bufio.NewReader(reader)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if c.TeeStdio && orig != nil {
				_, _ = io.WriteString(orig, line)
			}
			clean := strings.TrimRight(line, "\r\n")
			if clean != "" {
				c.Print(clean, lineType)
			}
		}
		if err != nil {
			break
		}
	}
}
