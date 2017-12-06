package xmlrpc

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"
	// "net/rpc"
	"reflect"
	"strings"
	"testing"
)

const xmlCallString = `<?xml version="1.0"?>
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

const xmlResponse = `<?xml version="1.0"?>
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

const xmlFault = `<methodResponse>
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

var xmlCallStruct = []interface{}{int(41), int(42), true,
	"árvíztűrő tükörfúrógép", -0.333333,
	// time.Date(1901, 2, 3, 4, 5, 6, 0, time.FixedZone("+0700", 7*3600)),
	[]byte{1, 2, 3, 5, 7, 11, 13, 17},
	map[string]interface{}{"k": "v"},
	[]interface{}{"a", "b", map[string]interface{}{"c": "d"}}, //map[string]interface{}{"p": 3, "q": 4},
	map[string]interface{}{"rune": "0x07", "string": "7"},
	"!last field!",
}

func TestMarshalCall(t *testing.T) {
	name, c, _, err := Unmarshal(bytes.NewBufferString(xmlCallString))
	if err != nil {
		t.Fatal("error unmarshaling xmlCall:", err)
	}
	t.Logf("unmarshalled call[%s]: %v\n", name, c)
}

func TestUnmarshalResponse(t *testing.T) {
	name, c, fault, err := Unmarshal(bytes.NewBufferString(xmlResponse))
	if err != nil {
		t.Fatal("error unmarshaling xmlResponse:", err)
	}
	t.Logf("unmarshalled response[%s]: %+v\n%v\n", name, c, fault)
}

func TestMarshalling(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	err := Marshal(buf, "trial", xmlCallStruct)
	if err != nil {
		t.Fatal("error marshalling xmlCallStruct:", err)
	}
	t.Logf("marshalled %+v\n:\n%s\n", xmlCallStruct, buf.Bytes())

	name, c, _, err := Unmarshal(buf)
	if err != nil {
		t.Fatal("cannot unmarshal previously marshalled struct:", err)
	}
	if name != "trial" {
		t.Error("name mismatch")
	}
	cS := fmt.Sprintf("%#v", c)
	// double check, because sometimes I got back array in array
	if !(fmt.Sprintf("%#v", xmlCallStruct) == cS ||
		fmt.Sprintf("%#v", []interface{}{xmlCallStruct}) == cS) {
		t.Errorf("struct mismatch:\n%s\n!=\n%#v\n&&\n%s\n!=\n%#v\n%v ? %v\n",
			cS, xmlCallStruct, cS, []interface{}{xmlCallStruct},
			reflect.DeepEqual(c, xmlCallStruct),
			reflect.DeepEqual(c, []interface{}{xmlCallStruct}))
	}
}

func TestFault(t *testing.T) {
	f := &Fault{Code: 4, Message: "Too many parameters."}
	buf := bytes.NewBuffer(nil)
	err := Marshal(buf, "", f)
	if err != nil {
		t.Fatal(fmt.Sprintf("error marshalling Fault: %s", err))
	}
	t.Logf("marshalled fault: %s", buf.Bytes())

	repl := func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}
	f1S := strings.Map(repl, buf.String())
	f2S := strings.Map(repl, xmlFault)
	if f1S != f2S {
		t.Errorf("fatal != constant\n%s\n!=\n%s", f1S, f2S)
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
	server.SetHTTPHandler(DefaultXMLRPCPath)
	l, e := net.Listen("tcp", ":1234")
	if e != nil {
		t.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)

	client, err := RPCDialHTTP("tcp", "localhost:1234")
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

func TestSimple(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		p := xml.NewDecoder(r.Body)
		se, _ := nextStart(p) // methodResponse
		if se.Name.Local != "methodCall" {
			http.Error(w, "missing methodCall", http.StatusBadRequest)
			return
		}
		se, _ = nextStart(p) // params
		if se.Name.Local != "methodName" {
			http.Error(w, "missing methodName", http.StatusBadRequest)
			return
		}
		se, _ = nextStart(p) // params
		if se.Name.Local != "params" {
			http.Error(w, "missing params", http.StatusBadRequest)
			return
		}
		var args []interface{}
		for {
			se, _ = nextStart(p) // param
			if se.Name.Local == "" {
				break
			}
			if se.Name.Local != "param" {
				http.Error(w, "missing param", http.StatusBadRequest)
				return
			}
			se, _ = nextStart(p) // value
			if se.Name.Local != "value" {
				http.Error(w, "missing value", http.StatusBadRequest)
				return
			}
			_, v, err := next(p)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			args = append(args, v)
		}

		if len(args) != 2 {
			http.Error(w, "bad number of arguments", http.StatusBadRequest)
			return
		}
		switch args[0].(type) {
		case int:
		default:
			http.Error(w, "args[0] should be int", http.StatusBadRequest)
			return
		}
		switch args[1].(type) {
		case int:
		default:
			http.Error(w, "args[1] should be int", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`
		<?xml version="1.0"?>
		<methodResponse>
		<params>
			<param>
				<value><int>` + fmt.Sprint(args[0].(int)+args[1].(int)) + `</int></value>
			</param>
		</params>
		</methodResponse>
		`))
	}))
	defer ts.Close()

	client := NewClient()
	v, _, err := client.Call(ts.URL+"/api", "add", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	i, ok := v.(int)
	if !ok {
		t.Fatalf("want int64 but got %T: %v", v, v)
	}
	if i != 3 {
		t.Fatalf("want 3 but got %v", v)
	}
}
func next(p *xml.Decoder) (xml.Name, interface{}, error) {
	se, e := nextStart(p)
	if e != nil {
		return xml.Name{}, nil, e
	}

	var nv interface{}
	switch se.Name.Local {
	case "string":
		var s string
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		return xml.Name{}, s, nil
	case "boolean":
		var s string
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		s = strings.TrimSpace(s)
		var b bool
		switch s {
		case "true", "1":
			b = true
		case "false", "0":
			b = false
		default:
			e = errors.New("invalid boolean value")
		}
		return xml.Name{}, b, e
	case "int", "i1", "i2", "i4", "i8":
		var s string
		var i int
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		i, e = strconv.Atoi(strings.TrimSpace(s))
		return xml.Name{}, i, e
	case "double":
		var s string
		var f float64
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		f, e = strconv.ParseFloat(strings.TrimSpace(s), 64)
		return xml.Name{}, f, e
	case "dateTime.iso8601":
		var s string
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		t, e := time.Parse("20060102T15:04:05", s)
		if e != nil {
			t, e = time.Parse("2006-01-02T15:04:05-07:00", s)
			if e != nil {
				t, e = time.Parse("2006-01-02T15:04:05", s)
			}
		}
		return xml.Name{}, t, e
	case "base64":
		var s string
		if e = p.DecodeElement(&s, &se); e != nil {
			return xml.Name{}, nil, e
		}
		if b, e := base64.StdEncoding.DecodeString(s); e != nil {
			return xml.Name{}, nil, e
		} else {
			return xml.Name{}, b, nil
		}
	case "member":
		nextStart(p)
		return next(p)
	case "value":
		nextStart(p)
		return next(p)
	case "name":
		nextStart(p)
		return next(p)
	case "struct":
		st := Struct{}

		se, e = nextStart(p)
		for e == nil && se.Name.Local == "member" {
			// name
			se, e = nextStart(p)
			if se.Name.Local != "name" {
				return xml.Name{}, nil, errors.New("invalid response")
			}
			if e != nil {
				break
			}
			var name string
			if e = p.DecodeElement(&name, &se); e != nil {
				return xml.Name{}, nil, e
			}
			se, e = nextStart(p)
			if e != nil {
				break
			}

			// value
			_, value, e := next(p)
			if se.Name.Local != "value" {
				return xml.Name{}, nil, errors.New("invalid response")
			}
			if e != nil {
				break
			}
			st[name] = value

			se, e = nextStart(p)
			if e != nil {
				break
			}
		}
		return xml.Name{}, st, nil
	case "array":
		var ar Array
		nextStart(p) // data
		for {
			nextStart(p) // top of value
			_, value, e := next(p)
			if e != nil {
				break
			}
			ar = append(ar, value)
		}
		return xml.Name{}, ar, nil
	}

	if e = p.DecodeElement(nv, &se); e != nil {
		return xml.Name{}, nil, e
	}
	return se.Name, nv, e
}
func nextStart(p *xml.Decoder) (xml.StartElement, error) {
	for {
		t, e := p.Token()
		if e != nil {
			return xml.StartElement{}, e
		}
		switch t := t.(type) {
		case xml.StartElement:
			return t, nil
		}
	}
	panic("unreachable")
}

type Array []interface{}
type Struct map[string]interface{}
