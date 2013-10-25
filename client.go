package xmlrpc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strings"
)

var debugClient = false

// xmlrpc
type clientCodec struct {
	conn     io.ReadWriteCloser
	response *rpc.Response
}

// NewClientCodec returns a new rpc.ClientCodec using XML-RPC on conn.
func NewClientCodec(conn io.ReadWriteCloser) rpc.ClientCodec {
	return &clientCodec{conn: conn}
}

func (c *clientCodec) WriteRequest(req *rpc.Request, param interface{}) error {
	var (
		err error
		arr []interface{}
		ok  bool
	)
	if arr, ok = param.([]interface{}); !ok {
		arr = []interface{}{param}
		log.Printf("param=%+v %T", param, param)
	}
	var w = io.Writer(c.conn)
	var buf *bytes.Buffer
	if debugClient {
		buf = bytes.NewBuffer(nil)
		w = io.MultiWriter(c.conn, buf)
	}
	err = Marshal(w, req.ServiceMethod, arr...)
	if debugClient {
		log.Printf("marshalled request %+v with error %s:\n%s", arr, err, buf)
	}
	return err
}

func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	c.response = r
	return nil
}

func (c *clientCodec) ReadResponseBody(dst interface{}) error {
	_, data, fault, err := Unmarshal(c.conn)
	if err != nil {
		return err
	}
	if fault != nil {
		c.response.Error = fault.String()
		return nil
	}
	if err = FillStruct(dst, data); err != nil {
		return fmt.Errorf("error reading %+v into %+v: %s", data, dst, err)
	}
	log.Printf("got response back: %+v (%s) %T", dst, dst, dst)
	if ptr, ok := dst.(*interface{}); ok {
		log.Printf("ptr=%+v", ptr)
		log.Printf("*ptr=%+v", *ptr)
	}
	return nil
}

func (c *clientCodec) Close() error {
	return c.conn.Close()
}

// NewClient returns a new Client to handle requests to the
// set of services at the other end of the connection.
// It adds a buffer to the write side of the connection so
// the header and payload are sent as a unit.
func NewClient(conn io.ReadWriteCloser) *rpc.Client {
	return rpc.NewClientWithCodec(NewClientCodec(conn))
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string) (*rpc.Client, error) {
	return DialHTTPPath(network, address, DefaultXMLRPCPath)
}

// DialHTTPPath connects to an HTTP RPC server
// at the specified network address and path.
func DialHTTPPath(network, address, path string) (*rpc.Client, error) {
	var err error
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && strings.HasPrefix(resp.Status, "200 ") {
		return NewClient(conn), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  network + " " + address,
		Addr: nil,
		Err:  err,
	}
}

// Dial connects to an RPC server at the specified network address.
func Dial(network, address string) (*rpc.Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), nil
}
