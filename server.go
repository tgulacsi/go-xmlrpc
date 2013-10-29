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

// DefaultXMLRPCPath is the path for the default handlers
var DefaultXMLRPCPath = "/xmlrpc"
var debugServer = false

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
	if fault, ok := getFault(param); ok {
		return Marshal(c.conn, "", fault)
	}
	arr, ok := param.([]interface{})
	if !ok {
		arr = []interface{}{param}
	}
	var (
		err error
		w   = io.Writer(c.conn)
	)
	if debugServer {
		buf := bytes.NewBuffer(nil)
		w = io.MultiWriter(c.conn, buf)
		defer func() {
			log.Printf("marshalled response %+v with error %s:\n%s",
				arr, err, buf)
		}()
	}
	log.Printf("marshaling %s %v into %s", req.ServiceMethod, arr, w)
	err = Marshal(w, req.ServiceMethod, arr...)
	return err
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	r.ServiceMethod, c.params, c.fault, err = Unmarshal(c.conn)
	log.Printf("RRH %s %+v %s", r.ServiceMethod, c.params, err)
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
	log.Printf("RRB got src=%+v, dst=%+v", src, dst)
	if err = FillStruct(dst, src); err != nil {
		return fmt.Errorf("error reading %+v into %+v: %s", src, dst, err)
	}

	return nil
}

func (c *serverCodec) Close() error {
	return c.conn.Close()
}

// XMLRpcServer is the server struct
type XMLRpcServer struct {
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
func (server *XMLRpcServer) ServeConn(conn io.ReadWriteCloser) {
	io.WriteString(conn, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	defer func() {
		if err := recover(); err != nil {
			log.Printf("ServeCodec failed: %s", err)
			Fault{Code: -1, Message: fmt.Sprintf("%s", err)}.WriteXML(conn)
			conn.Close()
		}
	}()
	srv := &serverCodec{conn: conn}
	server.ServeCodec(srv)
}

type readWriteCloser struct {
	io.ReadWriter
	io.Closer
}

type readWriter struct {
	io.Reader
	http.ResponseWriter
}

func (rw readWriter) Close() error {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (rw *readWriter) Write(p []byte) (int, error) {
	log.Printf("sending %q", p)
	return rw.ResponseWriter.Write(p)
}

// ServeHTTP implements an http.Handler that answers RPC req
func (server *XMLRpcServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Printf("connection from %s", req.RemoteAddr)
	if req.Method == "CONNECT" { // Go RPC
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
			return
		}
		io.WriteString(conn, "HTTP/1.0 200 Connected go Go XML-RPC server\r\n")
		io.WriteString(conn, "Content-Type: text/xml\r\n\r\n")
		server.ServeConn(readWriteCloser{buf, conn})
		return
	}
	// HTTP
	//w.Header().Set("Connection", "close")
	defer req.Body.Close()
	//w.Header().Set("Content-Length", "1000")
	w.Header().Set("Content-Type", "text/xml")
	//w.Header().Set("Transfer-Encoding", "chunked")
	server.ServeConn(&readWriter{Reader: req.Body, ResponseWriter: w})
	return
}

// DefaultServer is the default server
var DefaultServer = NewServer()

// NewServer returns a new XML-RPC server
func NewServer() *XMLRpcServer {
	return &XMLRpcServer{}
}

// SetHTTPHandler registers an HTTP handler for RPC messages on rpcPath,
// and a debugging handler on debugPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func (server *XMLRpcServer) SetHTTPHandler(rpcPath string) {
	log.Printf("rpcPath=%s", rpcPath)
	http.Handle(rpcPath, server)
	// http.Handle(debugPath, debugHTTP{server})
}
