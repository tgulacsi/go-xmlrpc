package xmlrpc

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var logger = log.New(os.Stderr, "xmlrpc", log.LstdFlags)

/// ISO8601 is not very much restrictive, so many combinations exist
const (
	// FullXMLRpcTime is the format of a full XML-RPC time
	FullXMLRpcTime = "2006-01-02T15:04:05-07:00"
	// LocalXMLRpcTime is the XML-RPC time without timezone
	LocalXMLRpcTime = "2006-01-02T15:04:05"
	// DenseXMLRpcTime is a dense-formatted local time
	DenseXMLRpcTime = "20060102T15:04:05"
	// DummyXMLRpcTime is seen in the wild
	DummyXMLRpcTime = "20060102T15:04:05-0700"
)

// SetLogger sets a new logger for this package, returns old logger
func SetLogger(lgr *log.Logger) *log.Logger {
	old := logger
	logger = lgr
	return old
}

// Unsupported is the error of "Unsupported type"
var Unsupported = errors.New("Unsupported type")

// Fault is the struct for the fault response
type Fault struct {
	Code    int
	Message string
}

func (f Fault) String() string {
	return fmt.Sprintf("%d: %s", f.Code, f.Message)
}
func (f Fault) Error() string {
	return f.String()
}

// WriteXML writes the XML representation of the fault into the Writer
func (f Fault) WriteXML(w io.Writer) (int, error) {
	return fmt.Fprintf(w, `<fault><value><struct>
			<member><name>faultCode</name><value><int>%d</int></value></member>
			<member><name>faultString</name><value><string>%s</string></value></member>
			</struct></value></fault>`, f.Code, xmlEscape(f.Message))
}

var xmlSpecial = map[byte]string{
	'<':  "&lt;",
	'>':  "&gt;",
	'"':  "&quot;",
	'\'': "&apos;",
	'&':  "&amp;",
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		if s, ok := xmlSpecial[c]; ok {
			b.WriteString(s)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

type valueNode struct {
	Type string `xml:",attr"`
	Body string `xml:",chardata"`
}

type state struct {
	p         *xml.Decoder
	level     int
	remainder *interface{}
	last      *xml.Token
}

func newParser(p *xml.Decoder) *state {
	return &state{p, 0, nil, nil}
}

const (
	tokStart = iota
	tokText
	tokStop
)

var (
	notStartElement = errors.New("not start element")
	nameMismatch    = errors.New("not the required token")
	notEndElement   = errors.New("not end element")
)

func (st *state) parseValue() (nv interface{}, e error) {
	var se xml.StartElement
	if se, e = st.getStart(""); e != nil {
		if ErrEq(e, notStartElement) {
			e = nil
		}
		return
	}

	// log.Printf("parseValue(%s)", se.Name.Local)
	var vn valueNode
	switch se.Name.Local {
	case "value":
		if nv, e = st.parseValue(); e == nil {
			// log.Printf("searching for /value")
			e = st.checkLast("value")
		}
		return
	case "boolean", "string", "int", "i1", "i2", "i4", "i8", "double", "dateTime.iso8601", "base64": //simple
		st.last = nil
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		switch se.Name.Local {
		case "boolean":
			nv, e = strconv.ParseBool(vn.Body)
		case "string":
			nv = vn.Body
		case "int", "i4":
			var i64 int64
			i64, e = strconv.ParseInt(vn.Body, 10, 32)
			nv = int(i64)
		case "double":
			nv, e = strconv.ParseFloat(vn.Body, 64)
		case "dateTime.iso8601":
			for _, format := range []string{FullXMLRpcTime, LocalXMLRpcTime, DenseXMLRpcTime, DummyXMLRpcTime} {
				nv, e = time.Parse(format, vn.Body)
				// log.Print("txt=", vn.Body, " t=", t, " fmt=", format, " e=", e)
				if e == nil {
					break
				}
			}
		case "base64":
			nv, e = base64.StdEncoding.DecodeString(vn.Body)
		}
		return

	case "struct":
		var name string
		values := make(map[string]interface{}, 4)
		nv = values
		for {
			// log.Printf("struct searching for member")
			if se, e = st.getStart("member"); e != nil {
				if ErrEq(e, notStartElement) {
					e = st.checkLast("struct")
					break
				}
				return
			}
			if name, e = st.getText("name"); e != nil {
				return
			}
			if se, e = st.getStart("value"); e != nil {
				return
			}
			if values[name], e = st.parseValue(); e != nil {
				return
			}
			if e = st.checkLast("value"); e != nil {
				log.Printf("didn't found last value element for struct member")
				return
			}
			if e = st.checkLast("member"); e != nil {
				return
			}
		}
		return

	case "array":
		values := make([]interface{}, 0, 4)
		var val interface{}
		// log.Printf("array searching for data")
		if _, e = st.getStart("data"); e != nil {
			return
		}
		for {
			if se, e = st.getStart("value"); e != nil {
				// log.Printf("array parsing ends with %s", e)
				if ErrEq(e, notStartElement) {
					e = nil //st.checkLast("data")
					break
				}
				return
			}
			if val, e = st.parseValue(); e != nil {
				return
			}
			values = append(values, val)
			if e = st.checkLast("value"); e != nil {
				log.Printf("didn't find value end for array")
				return
			}
		}
		if e = st.checkLast("data"); e == nil {
			e = st.checkLast("array")
		}
		nv = values
		return
	default:
		e = fmt.Errorf("cannot parse unknown tag %s", se)
	}
	return
}

func (st *state) token(typ int, name string) (t xml.Token, body string, e error) {
	// var ok bool
	if st.last != nil {
		t = *st.last
		st.last = nil
	}
Reading:
	for {
		if t != nil {
			switch t.(type) {
			case xml.StartElement:
				se := t.(xml.StartElement)
				if se.Name.Local != "" {
					break Reading
				}
			case xml.EndElement:
				ee := t.(xml.EndElement)
				if ee.Name.Local != "" {
					break Reading
				}
			default:
				// log.Printf("discarded %s %T", t, t)
			}
		}
		if t, e = st.p.Token(); e != nil {
			return
		}
		if t == nil {
			e = errors.New("nil token")
			return
		}
	}
	// log.Printf("token %s %T", t, t)
	switch typ {
	case tokStart, tokText:
		se, ok := t.(xml.StartElement)
		if !ok {
			// log.Printf("required startelement(%s), found %s %T", name, t, t)
			st.last = &t
			e = Errorf2(notStartElement, "required startelement(%s), found %s %T", name, t, t)
			return
		}
		switch typ {
		case tokStart:
			if name != "" && se.Name.Local != name {
				// log.Printf("required <%s>, found <%s>", name, se.Name.Local)
				e = Errorf2(nameMismatch, "required <%s>, found <%s>", name, se.Name.Local)
				return
			}
		default:
			var vn valueNode
			if e = st.p.DecodeElement(&vn, &se); e != nil {
				return
			}
			body = vn.Body
		}
	default:
		ee, ok := t.(xml.EndElement)
		if !ok {
			log.Printf("required endelement(%s), found %s %T", name, t, t)
			st.last = &t
			e = Errorf2(notEndElement, "required endelement(%s), found %s %T", name, t, t)
			return
		}
		if name != "" && ee.Name.Local != name {
			// log.Printf("required </%s>, found </%s>", name, ee.Name.Local)
			e = Errorf2(nameMismatch, "required </%s>, found </%s>", name, ee.Name.Local)
			return
		}
	}
	// log.Printf("  .")
	return
}

func (st *state) getStart(name string) (se xml.StartElement, e error) {
	var t xml.Token
	t, _, e = st.token(tokStart, name)
	se, _ = t.(xml.StartElement)
	if e != nil {
		return
	}
	se = t.(xml.StartElement)
	return
}

func (st *state) getText(name string) (text string, e error) {
	_, text, e = st.token(tokText, name)
	return
}

func (st *state) checkLast(name string) (e error) {
	_, _, e = st.token(tokStop, name)
	// log.Printf("  l")
	return
}

// call callse the method with "name" at the url location, with the given args
// returns the result, a fault pointer, and an error for communication errors
func call(client *http.Client, url, name string, args ...interface{}) (interface{}, *Fault, error) {
	req := bytes.NewBuffer(nil)
	req.WriteString(`<?xml version="1.0"?><methodCall>`)
	e := Marshal(req, name, args...)
	if e != nil {
		return nil, nil, e
	}
	r, e := http.Post(url, "text/xml", req)
	if e != nil {
		return nil, nil, e
	}
	defer r.Body.Close()
	// Since we do not always read the entire body, discard the rest, which
	// allows the http transport to reuse the connection.
	defer io.Copy(ioutil.Discard, r.Body)

	_, v, f, e := Unmarshal(r.Body)
	return v, f, e
}

// WriteXML writes v, typed if typ is true, into w Writer
func WriteXML(w io.Writer, v interface{}, typ bool) (err error) {
	logger.SetPrefix("WriteXML")
	var (
		r  reflect.Value
		ok bool
	)
	// go back from reflect.Value, if needed.
	if r, ok = v.(reflect.Value); !ok {
		r = reflect.ValueOf(v)
	} else {
		v = r.Interface()
	}
	if fp, ok := getFault(v); ok {
		_, err = fp.WriteXML(w)
		return
	}
	if b, ok := v.([]byte); ok {
		length := base64.StdEncoding.EncodedLen(len(b))
		dst := make([]byte, length)
		base64.StdEncoding.Encode(dst, b)
		_, err = taggedWrite(w, []byte("base64"), dst)
		return
	}
	if tim, ok := v.(time.Time); ok {
		_, err = taggedWriteString(w, "dateTime.iso8601", tim.Format(FullXMLRpcTime))
		return
	}
	t := r.Type()
	k := t.Kind()

	switch k {
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func:
		return Errorf2(Unsupported, "v=%#v t=%v k=%s", v, t, k)
	case reflect.Bool:
		_, err = fmt.Fprintf(w, "<boolean>%v</boolean>", v)
		return err
	case reflect.Int,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if typ {
			_, err = fmt.Fprintf(w, "<int>%v</int>", v)
			return err
		}
		_, err = fmt.Fprintf(w, "%v", v)
		return err
	case reflect.Float32, reflect.Float64:
		if typ {
			_, err = fmt.Fprintf(w, "<double>%v</double>", v)
			return err
		}
		_, err = fmt.Fprintf(w, "%v", v)
		return err
	case reflect.Array, reflect.Slice:
		if _, err = io.WriteString(w, "<array><data>\n"); err != nil {
			return
		}
		n := r.Len()
		for i := 0; i < n; i++ {
			if _, err = io.WriteString(w, "  <value>"); err != nil {
				return
			}
			if err = WriteXML(w, r.Index(i).Interface(), typ); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value>\n"); err != nil {
				return
			}
		}
		if _, err = io.WriteString(w, "</data></array>\n"); err != nil {
			return
		}
	case reflect.Interface:
		return WriteXML(w, r.Elem(), typ)
	case reflect.Map:
		if _, err = io.WriteString(w, "<struct>\n"); err != nil {
			return
		}
		for _, key := range r.MapKeys() {
			if _, err = io.WriteString(w, "  <member><name>"); err != nil {
				return
			}
			if _, err = io.WriteString(w, xmlEscape(key.Interface().(string))); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</name><value>"); err != nil {
				return
			}
			if err = WriteXML(w, r.MapIndex(key).Interface(), typ); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value></member>\n"); err != nil {
				return
			}
		}
		_, err = io.WriteString(w, "</struct>")
		return
	case reflect.Ptr:
		log.Printf("indirecting pointer v=%#v t=%v k=%s", v, t, k)
		return WriteXML(w, reflect.Indirect(r), typ)
	case reflect.String:
		if typ {
			_, err = fmt.Fprintf(w, "<string>%v</string>", xmlEscape(v.(string)))
			return
		}
		_, err = io.WriteString(w, xmlEscape(v.(string)))
		return
	case reflect.Struct:
		log.Printf("Struct %+v", v)
		if _, err = io.WriteString(w, "<struct>"); err != nil {
			return
		}
		n := r.NumField()
		for i := 0; i < n; i++ {
			c := t.Field(i).Name[:1]
			if strings.ToLower(c) == c { //have to skip unexported fields
				continue
			}
			if _, err = io.WriteString(w, "<member><name>"); err != nil {
				return
			}
			if _, err = io.WriteString(w, xmlEscape(t.Field(i).Name)); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</name><value>"); err != nil {
				return
			}
			if err = WriteXML(w, r.Field(i).Interface(), true); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value></member>"); err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, "</struct>")
		return
	case reflect.UnsafePointer:
		return WriteXML(w, r.Elem(), typ)
	}
	return
}

func taggedWrite(w io.Writer, tag, inner []byte) (n int, err error) {
	var j int
	for _, elt := range [][]byte{[]byte("<"), tag, []byte(">"), inner,
		[]byte("</"), tag, []byte(">")} {
		j, err = w.Write(elt)
		n += j
		if err != nil {
			return
		}
	}
	return
}
func taggedWriteString(w io.Writer, tag, inner string) (n int, err error) {
	if n, err = io.WriteString(w, "<"+tag+">"); err != nil {
		return
	}
	var j int
	j, err = io.WriteString(w, inner)
	n += j
	if err != nil {
		return
	}
	j, err = io.WriteString(w, "</"+tag+">")
	n += j
	return
}

// Marshal marshals the named thing (methodResponse if name == "", otherwise a methodCall)
// into the w Writer
func Marshal(w io.Writer, name string, args ...interface{}) (err error) {
	if name == "" {
		if _, err = io.WriteString(w, "<methodResponse>"); err != nil {
			return
		}
		if len(args) > 0 {
			fp, ok := getFault(args[0])
			// log.Printf("fault (%+v)? %s", args[0], ok)
			if ok {
				_, err = fp.WriteXML(w)
				if err == nil {
					_, err = io.WriteString(w, "\n</methodResponse>")
				}
				return
			}
		}
	} else {
		if _, err = io.WriteString(w, "<methodCall><methodName>"); err != nil {
			return
		}
		if _, err = io.WriteString(w, xmlEscape(name)); err != nil {
			return
		}
		if _, err = io.WriteString(w, "</methodName>\n"); err != nil {
			return
		}
	}
	if _, err = io.WriteString(w, "<params>\n"); err != nil {
		return
	}
	for _, arg := range args {
		if _, err = io.WriteString(w, "  <param><value>"); err != nil {
			return
		}
		if err = WriteXML(w, arg, true); err != nil {
			return
		}
		if _, err = io.WriteString(w, "</value></param>\n"); err != nil {
			return
		}
	}
	if name == "" {
		_, err = io.WriteString(w, "</params></methodResponse>")
	} else {
		_, err = io.WriteString(w, "</params></methodCall>")
	}
	return err
}

type Client struct {
	HttpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		HttpClient: &http.Client{Transport: http.DefaultTransport, Timeout: 10 * time.Second},
	}
}

func getFault(v interface{}) (*Fault, bool) {
	// log.Printf("getFault(%+v %T)", v, v)
	if f, ok := v.(Fault); ok {
		// log.Printf("  yes")
		return &f, true
	}
	if f, ok := v.(*Fault); ok {
		if f != nil {
			// log.Printf("  yes")
			return f, true
		}
	} else {
		if e, ok := v.(error); ok {
			return &Fault{Code: -1, Message: e.Error()}, true
		}
	}
	// log.Printf("  no")
	return nil, false
}

// Unmarshal unmarshals the thing (methodResponse, methodCall or fault),
// returns the name of the method call in the first return argument;
// the params of the call or the response
// or the Fault if this is a Fault
func Unmarshal(r io.Reader) (name string, params []interface{}, fault *Fault, e error) {
	p := xml.NewDecoder(r)
	st := newParser(p)
	typ := "methodResponse"
	if _, e = st.getStart(typ); ErrEq(e, nameMismatch) { // methodResponse or methodCall
		typ = "methodCall"
		if name, e = st.getText("methodName"); e != nil {
			return
		}
	}
	var se xml.StartElement
	if se, e = st.getStart("params"); e != nil {
		log.Printf("not params, but %s (%s)", se.Name.Local, e)
		if ErrEq(e, nameMismatch) && se.Name.Local == "fault" {
			var v interface{}
			if v, e = st.parseValue(); e != nil {
				return
			}
			fmap, ok := v.(map[string]interface{})
			if !ok {
				e = fmt.Errorf("fault not fault: %+v", v)
				return
			}
			fault = &Fault{Code: -1, Message: ""}
			code, ok := fmap["faultCode"]
			if !ok {
				e = fmt.Errorf("no faultCode in fault: %v", fmap)
				return
			}
			fcode, ok := code.(int)
			if !ok {
				e = fmt.Errorf("faultCode not int? %v", code)
				return
			}
			fault.Code = int(fcode)
			msg, ok := fmap["faultString"]
			if !ok {
				e = fmt.Errorf("no faultString in fault: %v", fmap)
				return
			}
			if fault.Message, ok = msg.(string); !ok {
				e = fmt.Errorf("faultString not strin? %v", msg)
				return
			}
			e = st.checkLast("fault")
		}
		return
	}
	params = make([]interface{}, 0, 8)
	var v interface{}
	for {
		if _, e = st.getStart("param"); e != nil {
			if ErrEq(e, notStartElement) {
				e = nil
				break
			}
			return
		}
		if v, e = st.parseValue(); e != nil {
			break
		}
		params = append(params, v)
		if e = st.checkLast("param"); e != nil {
			return
		}
	}
	if e = st.checkLast("params"); e == nil {
		e = st.checkLast(typ)
	}
	return
}

func (c *Client) Call(url, name string, args ...interface{}) (interface{}, *Fault, error) {
	return call(c.HttpClient, url, name, args...)
}

func Call(url, name string, args ...interface{}) (interface{}, *Fault, error) {
	return call(http.DefaultClient, url, name, args...)
}
