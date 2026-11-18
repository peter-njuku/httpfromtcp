package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadersParse(t *testing.T) {
	headers := NewHeaders()
	data := []byte("Host: localhost:42069\r\n\r\n")
	n, done, err := headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	host, ok := headers.Get("HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost:42069", host)
	assert.Equal(t, 25, n)
	assert.True(t, done)

	headers = NewHeaders()
	data = []byte("Host: localhost:42069\r\nFooFoo:     barbar\r\n\r\n")
	n, done, err = headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	host, ok = headers.Get("HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost:42069", host)
	foofoo, ok := headers.Get("FOOFOO")
	assert.True(t, ok)
	assert.Equal(t, "barbar", foofoo)
	assert.Equal(t, 45, n)
	assert.True(t, done)

	headers = NewHeaders()
	data = []byte("   Host: Localhost:42069     \r\n\r\n")
	n, done, err = headers.Parse(data)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, done)

	headers = NewHeaders()
	data = []byte("   Host       : Localhost:42069     \r\n\r\n")
	n, done, err = headers.Parse(data)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, done)

	headers = NewHeaders()
	data = []byte("H@st: Localhost:42069     \r\n\r\n")
	n, done, err = headers.Parse(data)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, done)

	headers = NewHeaders()
	data = []byte("Host: localhost:42069\r\nHost: localhost:42068\r\n")
	_, done, err = headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	host, ok = headers.Get("HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost:42069,localhost:42068", host)
	assert.False(t, done)

	headers = NewHeaders()
	data = []byte("Host: localhost:42069\r\n FooFoo:     barbar\r\n\r\n")
	_, _, err = headers.Parse(data)
	require.Error(t, err)
}

func TestMalformedHeader(t *testing.T) {
	_, _, err := parseHeader([]byte("Host localhost:42069\r\n\r\n"))
	require.Error(t, err)
}
