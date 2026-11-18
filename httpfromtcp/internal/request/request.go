package request

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/peter-njuku/httpfromtcp/internal/headers"
)

var (
	ErrorBadRequestLine         = fmt.Errorf("bad request line")
	ErrorUnsupportedHTTPVersion = fmt.Errorf("unsupported http version")
	Separator                   = []byte("\r\n")
	ErrorRequestInErrorState    = fmt.Errorf("Request in error satet")
)

type parserState string

const (
	StateInit    parserState = "init"
	StateHeaders parserState = "headers"
	StateDone    parserState = "done"
	StateBody    parserState = "body"
	StateError   parserState = "error"
)

type RequestLine struct {
	HTTPVersion   string
	RequestTarget string
	Method        string
}

func (rl *RequestLine) validHTTP() bool {
	return rl.HTTPVersion == "1.1"
}

type Request struct {
	RequestLine RequestLine
	Headers     *headers.Headers
	state       parserState
	Body        string
}

func NewRequest() *Request {
	return &Request{
		state:   StateInit,
		Headers: headers.NewHeaders(),
		Body:    "",
	}
}

func (r *Request) parse(data []byte) (int, error) {
	read := 0
outer:
	for {
		currentData := data[read:]
		switch r.state {
		case StateError:
			return 0, ErrorRequestInErrorState
		case StateInit:
			rl, n, err := parseRequestLine(currentData)
			if err != nil {
				r.state = StateError
				return 0, err
			}

			if n == 0 {
				break outer
			}
			r.RequestLine = *rl
			read += n

			r.state = StateHeaders

		case StateHeaders:
			n, done, err := r.Headers.Parse(currentData)
			if err != nil {
				r.state = StateError
				return read, err
			}
			if n == 0 && !done {
				break outer
			}
			read += n
			if done {
				length, err := getInt(r.Headers, "Content-Length", 0)
				if err != nil {
					r.state = StateError
					return read, err
				}
				if length > 0 {
					r.state = StateBody
				} else {
					r.state = StateDone
				}
			}

		case StateBody:
			length, err := getInt(r.Headers, "Content-Length", 0)
			//fmt.Println(length)
			if err != nil {
				r.state = StateError
				return read, err
			}
			if length == 0 {
				return read, ErrorBadRequestLine
			}

			remaining := min(length-len(r.Body), len(currentData))
			if remaining == 0 {
				break outer
			}
			r.Body += string(currentData[:remaining])
			//slog.Info("Parse#StateBody", "remaining", remaining, "read", read, "body", r.Body)

			read += remaining

			if len(r.Body) == length {
				r.state = StateDone
			}

		case StateDone:
			break outer
		default:
			panic("i have programmed poorly")
		}
	}
	return read, nil
}

func (r *Request) done() bool {
	return r.state == StateDone
}

func (r *Request) error() bool {
	return r.state == StateError
}

func parseRequestLine(b []byte) (*RequestLine, int, error) {
	idx := bytes.Index(b, Separator)
	if idx == -1 {
		return nil, 0, nil
	}

	startLine := b[:idx]
	read := idx + len(Separator)

	parts := bytes.Split(startLine, []byte(" "))
	//fmt.Println(string(parts[0]))
	if len(parts) != 3 || len(parts[0]) == 0 || len(parts[1]) == 0 || len(parts[2]) == 0 {
		return nil, 0, ErrorBadRequestLine
	}

	httpParts := bytes.Split(parts[2], []byte("/"))
	if len(httpParts) != 2 || len(httpParts[0]) == 0 || len(httpParts[1]) == 0 {
		return nil, 0, ErrorBadRequestLine
	}

	rl := &RequestLine{
		Method:        string(parts[0]),
		RequestTarget: string(parts[1]),
		HTTPVersion:   string(httpParts[1]),
	}

	if !rl.validHTTP() {
		return nil, 0, ErrorBadRequestLine
	}

	return rl, read, nil
}

func RequestFromReader(reader io.Reader) (*Request, error) {
	request := NewRequest()

	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)

	for !request.done() {
		n, err := reader.Read(tmp)

		if n > 0 {
			buf = append(buf, tmp[:n]...)
			readN, err := request.parse(buf)
			if err != nil {
				return nil, err
			}
			//copy(buf, buf[readN:bufLen])
			buf = buf[readN:]
		}

		if err != nil {
			if err == io.EOF {
				if request.done() {
					return request, nil
				}
				//slog.Info("Request from reader Unexpected EOF")
				return request, io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}

	return request, nil
}

func getInt(headers *headers.Headers, name string, defaultValue int) (int, error) {
	valueStr, exists := headers.Get(name)
	if !exists {
		//slog.Info("Get Int. Value string does not exist")
		return defaultValue, nil
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil || value < 0 {
		//slog.Info("Get Int", "strconv#Atoi", err)
		//slog.Info("Defaulting to 0...")

		return defaultValue, fmt.Errorf("requires a numerical value:%q", err)
	}
	return value, nil
}
