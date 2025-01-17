package nfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log"
	"net"
	"os"
	"time"
)

// Server is a handle to the listening NFS server.
type Server struct {
	Handler
	ID [8]byte
	context.Context
	debugLogger *log.Logger
	errorLogger *log.Logger
}

// RegisterMessageHandler registers a handler for a specific
// XDR procedure.
func RegisterMessageHandler(protocol uint32, proc uint32, handler HandleFunc) error {
	if registeredHandlers == nil {
		registeredHandlers = make(map[registeredHandlerID]HandleFunc)
	}
	for k := range registeredHandlers {
		if k.protocol == protocol && k.proc == proc {
			return errors.New("already registered")
		}
	}
	id := registeredHandlerID{protocol, proc}
	registeredHandlers[id] = handler
	return nil
}

// HandleFunc represents a handler for a specific protocol message.
type HandleFunc func(ctx context.Context, w *response, userHandler Handler) error

// TODO: store directly as a uint64 for more efficient lookups
type registeredHandlerID struct {
	protocol uint32
	proc     uint32
}

var registeredHandlers map[registeredHandlerID]HandleFunc

// Serve listens on the provided listener port for incoming client requests.
func (s *Server) Serve(l net.Listener) error {
	defer l.Close()
	baseCtx := context.Background()
	if s.Context != nil {
		baseCtx = s.Context
	}
	if bytes.Equal(s.ID[:], []byte{0, 0, 0, 0, 0, 0, 0, 0}) {
		if _, err := rand.Reader.Read(s.ID[:]); err != nil {
			return err
		}
	}

	var tempDelay time.Duration

	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			return err
		}
		tempDelay = 0
		c := s.newConn(conn)
		go c.serve(baseCtx)
	}
}

func (s *Server) newConn(nc net.Conn) *conn {
	c := &conn{
		Server:      s,
		Conn:        nc,
		debugLogger: s.debugLogger,
		errorLogger: s.errorLogger,
	}
	return c
}

// TODO: keep an immutable map for each server instance to have less
// chance of races.
func (s *Server) handlerFor(prog uint32, proc uint32) HandleFunc {
	for k, v := range registeredHandlers {
		if k.protocol == prog && k.proc == proc {
			return v
		}
	}
	return nil
}

// Serve is a singleton listener paralleling http.Serve
func Serve(l net.Listener, handler Handler, debugLogger *log.Logger, errorLogger *log.Logger) error {
	if errorLogger == nil {
		errorLogger = log.New(os.Stderr, "", log.Lshortfile)
	}
	if debugLogger == nil {
		debugLogger = log.New(os.Stdout, "", log.Lshortfile)
	}
	srv := &Server{Handler: handler, debugLogger: debugLogger, errorLogger: errorLogger}
	return srv.Serve(l)
}
