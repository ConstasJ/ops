package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"ops-go/pkg/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProbe_HTTP(t *testing.T) {
	t.Run("HTTP probe returns true on 200 OK", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer ts.Close()

		spec := &domain.ProbeSpec{HTTP: ts.URL}
		ok, msg := CheckProbe(context.Background(), spec)
		assert.True(t, ok)
		assert.Contains(t, msg, "200")
	})

	t.Run("HTTP probe returns false on 500 error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		spec := &domain.ProbeSpec{HTTP: ts.URL}
		ok, msg := CheckProbe(context.Background(), spec)
		assert.False(t, ok)
		assert.Contains(t, msg, "500")
	})

	t.Run("HTTP probe supports insecure TLS", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		// Secure should fail with unknown authority
		specSecure := &domain.ProbeSpec{HTTP: ts.URL, Insecure: false}
		ok, _ := CheckProbe(context.Background(), specSecure)
		assert.False(t, ok)

		// Insecure should succeed
		specInsecure := &domain.ProbeSpec{HTTP: ts.URL, Insecure: true}
		ok, msg := CheckProbe(context.Background(), specInsecure)
		assert.True(t, ok)
		assert.Contains(t, msg, "200")
	})

	t.Run("HTTP probe returns false on unreachable endpoint", func(t *testing.T) {
		spec := &domain.ProbeSpec{HTTP: "http://127.0.0.1:54321/nonexistent"}
		ok, msg := CheckProbe(context.Background(), spec)
		assert.False(t, ok)
		assert.Contains(t, msg, "failed")
	})
}

func TestCheckProbe_TCP(t *testing.T) {
	t.Run("TCP probe returns true on listening port", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer l.Close()

		spec := &domain.ProbeSpec{TCP: l.Addr().String()}
		ok, msg := CheckProbe(context.Background(), spec)
		assert.True(t, ok)
		assert.Contains(t, msg, "succeeded")
	})

	t.Run("TCP probe returns false on closed port", func(t *testing.T) {
		// Pick an unused port
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := l.Addr().String()
		_ = l.Close() // close immediately

		spec := &domain.ProbeSpec{TCP: addr}
		ok, msg := CheckProbe(context.Background(), spec)
		assert.False(t, ok)
		assert.Contains(t, msg, "failed")
	})
}

func TestCheckProbe_Nil(t *testing.T) {
	ok, msg := CheckProbe(context.Background(), nil)
	assert.True(t, ok)
	assert.Contains(t, msg, "no probe configured")
}
