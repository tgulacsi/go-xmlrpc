package xmlrpc

import (
	"io"
	"net/rpc"
)

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
	}
	err = Marshal(c.conn, req.ServiceMethod, arr...)
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
	if err = FillStruct(&dst, data); err != nil {
		return err
	}
	return nil
}

func (c *clientCodec) Close() error {
	return c.conn.Close()
}
