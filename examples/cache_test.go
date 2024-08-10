package examples

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
)

func TestCache(t *testing.T) {
	_, cache := policytesting.NewCache[net.Conn]()
	cachePolicy := cachepolicy.NewBuilder[net.Conn](cache).WithKey("connection").Build()

	// Get initial connection
	connection1, err := failsafe.Get[net.Conn](Connect, cachePolicy)
	assert.NoError(t, err)
	assert.NotNil(t, connection1.RemoteAddr())

	// Get cached connection
	connection2, err := failsafe.Get[net.Conn](Connect, cachePolicy)
	assert.NoError(t, err)
	assert.Same(t, connection1, connection2)
}

func Connect() (net.Conn, error) {
	return net.Dial("tcp", "golang.org:80")
}
