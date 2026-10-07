// SPDX-License-Identifier: GPL-3.0-or-later

// owneetctl is a small command-line client for owneetd, for scripts and tests.
//
//	owneetctl get /v1/status
//	owneetctl post /v1/power/restart
//	owneetctl put /v1/audio/volume '{"volume": 40}'
//	owneetctl delete /v1/network/wifi/Home
//	owneetctl events
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/fancy-dino/OwneetOS/daemon/internal/client"
	"github.com/fancy-dino/OwneetOS/daemon/internal/config"
	"github.com/fancy-dino/OwneetOS/daemon/internal/version"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: owneetctl [--socket PATH] COMMAND
  get PATH            GET an endpoint and print its JSON
  post PATH [JSON]    POST, with an optional JSON body
  put PATH JSON       PUT a JSON body
  delete PATH         DELETE
  events              print events as they arrive (Ctrl+C to stop)
  version             print the version
`)
	os.Exit(2)
}

func main() {
	socket := flag.String("socket", "", "owneetd socket (default $XDG_RUNTIME_DIR/owneetd.sock)")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	if args[0] == "version" {
		fmt.Println("owneetctl", version.Version)
		return
	}
	path := *socket
	if path == "" {
		var err error
		if path, err = config.DefaultSocket(); err != nil {
			fail(err)
		}
	}
	c := client.New(path)

	switch cmd := args[0]; cmd {
	case "events":
		stream(c)
	case "get", "delete", "post", "put":
		if len(args) < 2 || !strings.HasPrefix(args[1], "/") {
			usage()
		}
		var body io.Reader
		if len(args) > 2 {
			if !json.Valid([]byte(args[2])) {
				fail(fmt.Errorf("body is not valid JSON: %s", args[2]))
			}
			body = strings.NewReader(args[2])
		} else if cmd == "put" {
			usage()
		}
		os.Exit(request(c, strings.ToUpper(cmd), args[1], body))
	default:
		usage()
	}
}

// request performs one call and prints the response; the exit code is 0 for 2xx, 1 otherwise.
func request(c *http.Client, method, path string, body io.Reader) int {
	req, err := http.NewRequest(method, "http://owneetd"+path, body)
	if err != nil {
		fail(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		fail(fmt.Errorf("cannot reach owneetd: %w", err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var pretty bytes.Buffer
	if json.Indent(&pretty, data, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else if len(data) > 0 {
		fmt.Println(string(data))
	}
	if resp.StatusCode >= 300 {
		return 1
	}
	return 0
}

func stream(c *http.Client) {
	resp, err := c.Get("http://owneetd/v1/events")
	if err != nil {
		fail(fmt.Errorf("cannot reach owneetd: %w", err))
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			fmt.Println(line)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "owneetctl:", err)
	os.Exit(1)
}
