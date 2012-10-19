package xmlrpc

import (
	"io"
	"net/rpc"
	"reflect"
)

// xmlrpc
type serverCodec struct {
	conn   io.ReadWriteCloser
	params []interface{} //parameters
	fault  *Fault
}

// NewServerCodec returns a new rpc.ServerCodec using XML-RPC on conn.
func NewServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &serverCodec{conn: conn}
}

func (c *serverCodec) WriteResponse(req *rpc.Response, param interface{}) error {
	var (
		err error
	)
	fault, ok := param.(*Fault)
	if ok {
		err = Marshal(c.conn, "", fault)
	} else {
		arr, ok := param.([]interface{})
		if !ok {
			arr = []interface{}{param}
		}
		err = Marshal(c.conn, req.ServiceMethod, arr...)
	}
	return err
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	r.ServiceMethod, c.params, c.fault, err = Unmarshal(c.conn)
	if err != nil {
		return err
	}
	return nil
}

func (c *serverCodec) ReadRequestBody(dst interface{}) (err error) {
	if dst == nil {
		return nil
	}
	// XML-RPC params is array value.
	// RPC params is struct.
	// Should think about making RPC more general.
	var src interface{} = c.params
	if len(c.params) == 1 {
		t := reflect.TypeOf(c.params[0])
		if t.Kind() == reflect.Struct {
			src = c.params[0]
		}
	}
	if err = FillStruct(&dst, src); err != nil {
		return err
	}

	return nil
}

func (c *serverCodec) Close() error {
	return c.conn.Close()
}
