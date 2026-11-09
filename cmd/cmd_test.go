package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd := cmd.NewRootCommand()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "ops-go")
	assert.Contains(t, out, "Version:")
}

func TestPingCommand_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_ping", "/v1.56/_ping":
			w.Header().Set("API-Version", "1.56")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/v1.56/version", "/version":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Version":"29.9.2","ApiVersion":"1.56","GoVersion":"go1.26.10","Os":"linux","Arch":"amd64"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	buf := new(bytes.Buffer)
	rootCmd := cmd.NewRootCommand()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"ping", "--socket", server.URL})

	err := rootCmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "29.9.2")
	assert.Contains(t, out, "1.56")
}

func TestPingCommand_OfflineServer(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd := cmd.NewRootCommand()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"ping", "--socket", "http://127.0.0.1:59999"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}
