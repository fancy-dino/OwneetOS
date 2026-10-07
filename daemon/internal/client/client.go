// SPDX-License-Identifier: GPL-3.0-or-later

// Package client talks to owneetd over its Unix socket.
package client

import (
	"context"
	"net"
	"net/http"
)

// New returns an HTTP client whose requests go to the Unix socket at path, whatever the host
// in the URL. Use URLs like "http://owneetd/v1/status".
func New(path string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
		},
	}
}
