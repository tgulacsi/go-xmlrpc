package xmlrpc

import (
	"io"
	"net/rpc"
)

// xmlrpc
type clientCodec struct {
	conn io.ReadWriteCloser
}

// NewClientCodec returns a new rpc.ClientCodec using JSON-RPC on conn.
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
	}
	err = Marshal(c.conn, req.ServiceMethod, arr)
	return err
}

func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	return nil
}

func (c *clientCodec) ReadResponseBody(dst interface{}) error {
	// methodName, data, err, fault := Unmarshal(c.conn)
	return nil
}

func (c *clientCodec) Close() error {
	return c.conn.Close()
}
