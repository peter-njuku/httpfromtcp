package request

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chunkReader struct {
	data            string
	numBytesPerRead int
	pos             int
}

func (cr *chunkReader) Read(p []byte) (n int, err error) {
	if cr.pos >= len(cr.data) {
		return 0, io.EOF
	}

	endIdx := cr.pos + cr.numBytesPerRead
	if endIdx > len(cr.data) {
		endIdx = len(cr.data)
	}

	n = copy(p, cr.data[cr.pos:endIdx])
	cr.pos += n
	//if n > cr.numBytesPerRead {
	//	n = cr.numBytesPerRead
	//	cr.pos = n - cr.numBytesPerRead
	//}
	return n, nil
}

func TestRequestLineParse_EdgeCases(t *testing.T) {
	// Empty input
	_, err := RequestFromReader(strings.NewReader(""))
	require.Error(t, err)

	// Only CRLF
	_, err = RequestFromReader(strings.NewReader("\r\n"))
	require.Error(t, err)

	// Missing version
	_, err = RequestFromReader(strings.NewReader("GET /\r\n\r\n"))
	require.Error(t, err)

	// Missing target
	_, err = RequestFromReader(strings.NewReader("GET  HTTP/1.1\r\n\r\n"))
	require.Error(t, err)

	// Missing method
	_, err = RequestFromReader(strings.NewReader(" / HTTP/1.1\r\n\r\n"))
	require.Error(t, err)

	// Too many parts in request line
	_, err = RequestFromReader(strings.NewReader("GET / HTTP/1.1 extra\r\n\r\n"))
	require.Error(t, err)

	// Malformed version (no slash)
	_, err = RequestFromReader(strings.NewReader("GET / HTTP1.1\r\n\r\n"))
	require.Error(t, err)

	// Request line only, no terminating blank line
	_, err = RequestFromReader(strings.NewReader("GET / HTTP/1.1\r\n"))
	require.Error(t, err)

	// Target with query string
	reader := &chunkReader{
		data:            "GET /foo?bar=baz HTTP/1.1\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "/foo?bar=baz", r.RequestLine.RequestTarget)

	// OPTIONS with asterisk target
	reader = &chunkReader{
		data:            "OPTIONS * HTTP/1.1\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "OPTIONS", r.RequestLine.Method)
	assert.Equal(t, "*", r.RequestLine.RequestTarget)

	// Long target
	long := "/" + strings.Repeat("a", 10000)
	reader = &chunkReader{
		data:            "GET " + long + " HTTP/1.1\r\n\r\n",
		numBytesPerRead: 7,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, long, r.RequestLine.RequestTarget)
}

func TestParseHeaders_EdgeCases(t *testing.T) {
	// No headers at all
	reader := &chunkReader{
		data:            "GET / HTTP/1.1\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	_, ok := r.Headers.Get("HOST")
	assert.False(t, ok)

	// Header with empty value
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\nX-Empty:\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	v, ok := r.Headers.Get("X-EMPTY")
	assert.True(t, ok)
	assert.Equal(t, "", v)

	// Header value with leading/trailing OWS
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\nHost:    localhost   \r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	v, ok = r.Headers.Get("HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost", v)

	// Empty header name
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\n: value\r\n\r\n",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(reader)
	require.Error(t, err)

	// Header name with space
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\nBad Header: value\r\n\r\n",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(reader)
	require.Error(t, err)

	// Duplicate headers
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\nX-Dup: one\r\nX-Dup: two\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	v, ok = r.Headers.Get("X-DUP")
	assert.True(t, ok)
	assert.NotEmpty(t, v)

	// Headers split across every chunk boundary 1..7
	for n := 1; n <= 7; n++ {
		reader = &chunkReader{
			data:            "GET / HTTP/1.1\r\nHost: localhost:42069\r\nX-Test: abc\r\n\r\n",
			numBytesPerRead: n,
		}
		r, err = RequestFromReader(reader)
		require.NoError(t, err)
		require.NotNil(t, r)
		v, ok = r.Headers.Get("X-TEST")
		assert.True(t, ok)
		assert.Equal(t, "abc", v)
	}
}

func TestParseBody_EdgeCases(t *testing.T) {
	// Content-Length: 0
	reader := &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: 0\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Empty(t, r.Body)

	// Body longer than Content-Length is truncated
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello world",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "hello", string(r.Body))

	// Non-numeric Content-Length
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: abc\r\n\r\nhi",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(reader)
	require.Error(t, err)

	// Negative Content-Length
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: -5\r\n\r\nhi",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(reader)
	require.Error(t, err)

	// Body containing CRLF
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: 12\r\n\r\nline1\r\nline2",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "line1\r\nline2", string(r.Body))

	// Body containing null bytes
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\na\x00b\x00c",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "a\x00b\x00c", string(r.Body))

	// Body split across every chunk boundary 1..10
	body := "The quick brown fox jumps over the lazy dog"
	for n := 1; n <= 10; n++ {
		reader = &chunkReader{
			data:            "POST / HTTP/1.1\r\nContent-Length: 43\r\n\r\n" + body,
			numBytesPerRead: n,
		}
		r, err = RequestFromReader(reader)
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, body, string(r.Body))
	}

	// Case-insensitive Content-Length
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\ncontent-LENGTH: 2\r\n\r\nhi",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "hi", string(r.Body))

	// Large body
	large := strings.Repeat("x", 100000)
	reader = &chunkReader{
		data:            "POST / HTTP/1.1\r\nContent-Length: 100000\r\n\r\n" + large,
		numBytesPerRead: 7,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, large, string(r.Body))
}

func TestChunkReader_Read(t *testing.T) {
	// Read all with 1-byte chunk size
	cr := &chunkReader{data: "hello world", numBytesPerRead: 1}
	var out []byte
	buf := make([]byte, 32)
	for {
		n, err := cr.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
	assert.Equal(t, "hello world", string(out))

	// Read buffer smaller than chunk size
	cr = &chunkReader{data: "abcdef", numBytesPerRead: 4}
	buf = make([]byte, 2)
	n, err := cr.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, "ab", string(buf[:n]))

	// Empty data returns EOF
	cr = &chunkReader{data: "", numBytesPerRead: 3}
	buf = make([]byte, 10)
	n, err = cr.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	// Chunk size larger than remaining data
	cr = &chunkReader{data: "abc", numBytesPerRead: 100}
	buf = make([]byte, 100)
	n, err = cr.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, "abc", string(buf[:n]))

	n, err = cr.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestRequestFromReader_Integration(t *testing.T) {
	// Full request read 1 byte at a time
	raw := "POST /submit?x=1 HTTP/1.1\r\n" +
		"Host: localhost:42069\r\n" +
		"User-Agent: curl/8.14.6\r\n" +
		"Accept: */*\r\n" +
		"Content-Length: 11\r\n" +
		"\r\n" +
		"hello world"
	reader := &chunkReader{data: raw, numBytesPerRead: 1}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "POST", r.RequestLine.Method)
	assert.Equal(t, "/submit?x=1", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HTTPVersion)
	host, ok := r.Headers.Get("HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost:42069", host)
	ua, ok := r.Headers.Get("USER-AGENT")
	assert.True(t, ok)
	assert.Equal(t, "curl/8.14.6", ua)
	accept, ok := r.Headers.Get("ACCEPT")
	assert.True(t, ok)
	assert.Equal(t, "*/*", accept)
	assert.Equal(t, "hello world", string(r.Body))

	// Missing final CRLF terminator
	reader = &chunkReader{
		data:            "GET / HTTP/1.1\r\nHost: x",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(reader)
	require.Error(t, err)

	// Unknown method token
	reader = &chunkReader{
		data:            "FOOBAR / HTTP/1.1\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, string("FOOBAR"), r.RequestLine.Method)
}
