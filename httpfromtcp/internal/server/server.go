package server

import (
	"fmt"
	"io"
	"net"

	"github.com/peter-njuku/httpfromtcp/internal/request"
	"github.com/peter-njuku/httpfromtcp/internal/response"
)

type HandlerError struct {
	StatusCode response.StatusCode
	Message    string
}
type Handler func(w *response.Writer, req *request.Request)

type Server struct {
	listener net.Listener
	handler  Handler
}

func (s *Server) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func runConnection(s *Server, conn io.ReadWriteCloser) {
	defer conn.Close()

	responseWriter := response.NewWriter(conn)

	r, err := request.RequestFromReader(conn)
	if err != nil {
		responseWriter.WriteStatusLine(response.StatusBadRequest)
		responseWriter.WriteHeaders(response.GetDefaultHeaders(0))
		return
	}

	s.handler(responseWriter, r)
	//if handlerError != nil {
	//	body := []byte(handlerError.Message)
	//	err := response.GetDefaultHeaders(len(body))
	//	response.WriteStatusLine(conn, handlerError.StatusCode)
	//	response.WriteHeaders(conn, err)
	//	conn.Write([]byte(handlerError.Message))
	//	return
	//}

	//body := writer.Bytes()
	//headers.Replace("Content-Length", fmt.Sprintf("%d", len(body)))
	//response.WriteStatusLine(conn, response.StatusOK)
	//response.WriteHeaders(conn, headers)
	//conn.Write(body)
}

func runServer(s *Server, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		go runConnection(s, conn)
	}
}

func Serve(port uint16, handler Handler) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	server := &Server{
		handler: handler,
	}
	go runServer(server, listener)
	return server, nil
}
