// file: internal/mtls/bridge.go
// version: 1.1.0

package mtls

import (
	"io"
	"net"
	"sync"
)

// halfCloser is implemented by connections that support closing only the
// write side (e.g. *net.TCPConn), enabling graceful half-close semantics.
type halfCloser interface {
	CloseWrite() error
}

// closeWrite closes only the write direction of conn if supported, otherwise
// falls back to a full Close.
func closeWrite(conn net.Conn) {
	if hc, ok := conn.(halfCloser); ok {
		hc.CloseWrite() //nolint:errcheck
	} else {
		conn.Close()
	}
}

// closeIfCloser closes w if it implements io.Closer; used to signal EOF to a
// subprocess stdin pipe so the subprocess can finish and its stdout can drain.
func closeIfCloser(w io.Writer) {
	if c, ok := w.(io.Closer); ok {
		c.Close() //nolint:errcheck
	}
}

// Bridge copies data bidirectionally between a network connection and
// a subprocess's stdout (read) and stdin (write).
//
// When the connection closes (remote EOF), Bridge closes subStdin (if it
// implements io.Closer) so the subprocess can detect end-of-input and its
// stdout pipe drains naturally.
func Bridge(conn net.Conn, subStdout io.Reader, subStdin io.Writer) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(2)

	// conn → subprocess stdin
	go func() {
		defer wg.Done()
		_, err := io.Copy(subStdin, conn)
		// Signal EOF to the subprocess so it can flush and close subStdout.
		closeIfCloser(subStdin)
		errs <- err
	}()

	// subprocess stdout → conn
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, subStdout)
		// Once subStdout is drained, close conn so the remote side sees EOF.
		conn.Close()
		errs <- err
	}()

	err := <-errs
	conn.Close()
	wg.Wait()

	if err == io.EOF {
		return nil
	}
	return err
}

// BridgeStdio copies data bidirectionally between a network connection and
// local stdin/stdout. It uses half-close semantics when available so that
// sending all of stdin's data (and signalling EOF to the remote) does not
// prevent the remaining response from being read.
func BridgeStdio(conn net.Conn, stdin io.Reader, stdout io.Writer) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(2)

	// stdin → conn (then half-close so remote sees EOF on its read side)
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, stdin)
		// Half-close the write side of conn; the read side stays open so
		// goroutine 2 can still receive the server's response.
		closeWrite(conn)
		errs <- err
	}()

	// conn → stdout (reads until remote closes or conn is fully closed)
	go func() {
		defer wg.Done()
		_, err := io.Copy(stdout, conn)
		errs <- err
	}()

	err := <-errs
	conn.Close()
	wg.Wait()

	if err == io.EOF {
		return nil
	}
	return err
}
