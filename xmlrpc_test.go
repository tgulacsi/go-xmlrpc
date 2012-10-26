package xmlrpc

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	// "net/rpc"
	"reflect"
	"strings"
	"testing"
)

const XmlCallString = `<?xml version="1.0"?>
<methodCall>
   <methodName>examples.getStateName</methodName>
   <params>
      <param><value><i4>41</i4></value></param>
      <param><value><int>42</int></value></param>
      <param><value><boolean>1</boolean></value></param>
      <param><value><string>árvízűtő tükörfúrógép</string></value></param>
      <param><value><double>-0.333333</double></value></param>
      <param><value><dateTime.iso8601>19980717T14:08:55</dateTime.iso8601></value></param>
      <param><value><base64>eW91IGNhbid0IHJlYWQgdGhpcyE=</base64></value></param>
      </params>
   </methodCall>`

const XmlResponse = `<?xml version="1.0"?>
<methodResponse>
   <params>
      <param><value><dateTime.iso8601>19980717T14:08:55</dateTime.iso8601></value></param>
      <param><value><i4>41</i4></value></param>
      <param><value><int>42</int></value></param>
      <param><value><boolean>1</boolean></value></param>
      <param><value><string>árvízűrő tükörfúrógép</string></value></param>
      <param><value><double>-0.333333</double></value></param>
      <param><value><base64>eW91IGNhbid0IHJlYWQgdGhpcyE=</base64></value></param>
      <param><value><string>!last param!</string></value></param>
      </params>
   </methodResponse>`

const XmlFault = `<methodResponse>
   <fault>
      <value>
         <struct>
            <member>
               <name>faultCode</name>
               <value><int>4</int></value>
               </member>
            <member>
               <name>faultString</name>
               <value><string>Too many parameters.</string></value>
               </member>
            </struct>
         </value>
      </fault>
   </methodResponse>`

var XmlCallStruct = []interface{}{int(41), int(42), true,
	"árvíztűrő tükörfúrógép", -0.333333,
	// time.Date(1901, 2, 3, 4, 5, 6, 0, time.FixedZone("+0700", 7*3600)),
	[]byte{1, 2, 3, 5, 7, 11, 13, 17},
	map[string]interface{}{"k": "v"},
	[]interface{}{"a", "b", map[string]interface{}{"c": "d"}}, //map[string]interface{}{"p": 3, "q": 4},
	map[string]interface{}{"rune": "0x07", "string": "7"},
	"!last field!",
}

func TestMarshalCall(t *testing.T) {
	name, c, _, err := Unmarshal(bytes.NewBufferString(XmlCallString))
	if err != nil {
		t.Fatal("error unmarshaling XmlCall:", err)
	}
	t.Logf("unmarshalled call[%s]: %v\n", name, c)
}

func TestUnmarshalResponse(t *testing.T) {
	name, c, fault, err := Unmarshal(bytes.NewBufferString(XmlResponse))
	if err != nil {
		t.Fatal("error unmarshaling XmlResponse:", err)
	}
	t.Logf("unmarshalled response[%s]: %+v\n%s\n", name, c, fault)
}

func TestMarshalling(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	err := Marshal(buf, "trial", XmlCallStruct)
	if err != nil {
		t.Fatal("error marshalling XmlCallStruct:", err)
	}
	t.Logf("marshalled %+v\n:\n%s\n", XmlCallStruct, buf.Bytes())

	name, c, _, err := Unmarshal(buf)
	if err != nil {
		t.Fatal("cannot unmarshal previously marshalled struct:", err)
	}
	if name != "trial" {
		t.Error("name mismatch")
	}
	c_s := fmt.Sprintf("%#v", c)
	// double check, because sometimes I got back array in array
	if !(fmt.Sprintf("%#v", XmlCallStruct) == c_s ||
		fmt.Sprintf("%#v", []interface{}{XmlCallStruct}) == c_s) {
		t.Errorf("struct mismatch:\n%s\n!=\n%#v\n&&\n%s\n!=\n%#v\n%s ? %s\n",
			c_s, XmlCallStruct, c_s, []interface{}{XmlCallStruct},
			reflect.DeepEqual(c, XmlCallStruct),
			reflect.DeepEqual(c, []interface{}{XmlCallStruct}))
	}
}

func TestFault(t *testing.T) {
	f := &Fault{Code: 4, Message: "Too many parameters."}
	buf := bytes.NewBuffer(nil)
	err := Marshal(buf, "", f)
	if err != nil {
		t.Fatal("error marshalling Fault: %s", err)
	}
	t.Logf("marshalled fault: %s", buf.Bytes())

	repl := func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}
	f1_s := strings.Map(repl, buf.String())
	f2_s := strings.Map(repl, XmlFault)
	if f1_s != f2_s {
		t.Errorf("fatal != constant\n%s\n!=\n%s", f1_s, f2_s)
	}

	_, _, f2, err := Unmarshal(bytes.NewBuffer(buf.Bytes()))
	if err != nil {
		t.Fatalf("cannot unmarshal previously marshalled fault (\n%s\n):%s",
			buf, err)
	}
	if f2.String() != f.String() {
		t.Errorf("f1=%s != f2=%s", f, f2)
	}
}

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}
func TestClientServer(t *testing.T) {
	debugServer, debugClient = true, true
	arith := new(Arith)
	server := NewServer()
	server.Register(arith)
	server.HandleHTTP(DefaultXMLRPCPath)
	l, e := net.Listen("tcp", ":1234")
	if e != nil {
		t.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)

	client, err := DialHTTP("tcp", "localhost:1234")
	if err != nil {
		t.Fatal("dialing:", err)
	}
	// Synchronous call
	args := &Args{7, 8}
	// var reply int
	// err = client.Call("Arith.Multiply", args, &reply)
	reply := new(Quotient)
	err = client.Call("Arith.Divide", args, reply)
	if err != nil {
		t.Fatal("arith error:", err)
	}
	t.Logf("Arith: %d*%d=%+v", args.A, args.B, reply)
}
