package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"ops-go/pkg/domain"
)

// CheckProbe checks an HTTP or TCP probe endpoint. Returns true if healthy.
func CheckProbe(ctx context.Context, p *domain.ProbeSpec) (bool, string) {
	if p == nil || (p.HTTP == "" && p.TCP == "") {
		return true, "no probe configured"
	}

	if p.HTTP != "" {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
		}
		if p.Insecure {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.HTTP, nil)
		if err != nil {
			return false, fmt.Sprintf("invalid probe HTTP URL %s: %v", p.HTTP, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, fmt.Sprintf("HTTP probe failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true, fmt.Sprintf("HTTP probe %s returned %d", p.HTTP, resp.StatusCode)
		}
		return false, fmt.Sprintf("HTTP probe %s returned status %d", p.HTTP, resp.StatusCode)
	}

	if p.TCP != "" {
		dialer := &net.Dialer{
			Timeout: 3 * time.Second,
		}
		conn, err := dialer.DialContext(ctx, "tcp", p.TCP)
		if err != nil {
			return false, fmt.Sprintf("TCP dial %s failed: %v", p.TCP, err)
		}
		_ = conn.Close()
		return true, fmt.Sprintf("TCP dial %s succeeded", p.TCP)
	}

	return true, "no probe configured"
}
