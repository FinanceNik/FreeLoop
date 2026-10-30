package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client talks to a running app's API.
type Client struct {
	mu   sync.Mutex
	c    net.Conn
	sc   *bufio.Scanner
	next int
}

// Dial connects to the socket at path.
func Dial(path string) (*Client, error) {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("Freeloop is not running (no API at %s)", path)
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	return &Client{c: c, sc: sc}, nil
}

// Close ends the connection.
func (c *Client) Close() error { return c.c.Close() }

// Call sends a request and decodes its result into out (which may be nil).
func (c *Client) Call(method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := c.c.Write(append(b, '\n')); err != nil {
		return err
	}
	if !c.sc.Scan() {
		if err := c.sc.Err(); err != nil {
			return err
		}
		return fmt.Errorf("the app closed the connection")
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(c.sc.Bytes(), &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}
