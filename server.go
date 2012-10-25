package xmlrpc

import (
	// "bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"reflect"
)

var DefaultXMLRPCPath = "/xmlrpc"
var debugServer bool = false

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
		var w io.Writer = c.conn
		var buf *bytes.Buffer
		if debugServer {
			buf = bytes.NewBuffer(nil)
			w = io.MultiWriter(c.conn, buf)
		}
		err = Marshal(w, req.ServiceMethod, arr...)
		if debugServer {
			log.Printf("marshalled response %+v with error %s:\n%s",
				arr, err, buf)
		}
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
		return fmt.Errorf("error reading %+v into %+v: %s", src, dst, err)
	}

	return nil
}

func (c *serverCodec) Close() error {
	return c.conn.Close()
}

type XmlRpcServer struct {
	rpc.Server
}

// ServeConn runs the XML-RPC server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
func ServeConn(conn io.ReadWriteCloser) {
	rpc.ServeCodec(NewServerCodec(conn))
}

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
// ServeConn uses the gob wire format (see package gob) on the
// connection.  To use an alternate codec, use ServeCodec.
func (server *XmlRpcServer) ServeConn(conn io.ReadWriteCloser) {
	// buf := bufio.NewWriter(conn)
	srv := &serverCodec{conn: conn}
	server.ServeCodec(srv)
}

// ServeHTTP implements an http.Handler that answers RPC req
func (server *XmlRpcServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Printf("connection from %s", req.RemoteAddr)
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	io.WriteString(conn, "HTTP/1.0 200 Connected go Go XML-RPC server\n\n")
	server.ServeConn(conn)
}

var DefaultServer = NewServer()

func NewServer() *XmlRpcServer {
	return &XmlRpcServer{}
}

// HandleHTTP registers an HTTP handler for RPC messages on rpcPath,
// and a debugging handler on debugPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func (server *XmlRpcServer) HandleHTTP(rpcPath string) {
	log.Printf("rpcPath=%s", rpcPath)
	http.Handle(rpcPath, server)
	// http.Handle(debugPath, debugHTTP{server})
}
