package xmlrpc

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var logger = log.New(os.Stderr, "xmlrpc", log.LstdFlags)

const (
	FullXmlRpcTime  = "2006-01-02T15:04:05-07:00"
	LocalXmlRpcTime = "2006-01-02T15:04:05"
	DenseXmlRpcTime = "20060102T15:04:05"
)

// sets new logger for this package, returns old logger
func SetLogger(lgr *log.Logger) *log.Logger {
	old := logger
	logger = lgr
	return old
}

var Unsupported = errors.New("Unsupported type")
var levelDecremented = errors.New("level decremented")

// type Array []interface{}
// type Struct map[string]interface{}
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
		if e == notStartElement {
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
			nv, e = strconv.ParseInt(vn.Body, 10, 64)
		case "double":
			nv, e = strconv.ParseFloat(vn.Body, 64)
		case "dateTime.iso8601":
			for _, format := range []string{FullXmlRpcTime, LocalXmlRpcTime, DenseXmlRpcTime} {
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
				if e == notStartElement {
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
				if e == notStartElement {
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
		switch t.(type) {
		case xml.StartElement, xml.EndElement:
			break Reading
		default:
			// log.Printf("discarded %s %T", t, t)
		}
		if t, e = st.p.Token(); e != nil {
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
			e = notStartElement
			return
		}
		switch typ {
		case tokStart:
			if name != "" && se.Name.Local != name {
				// log.Printf("required <%s>, found <%s>", name, se.Name.Local)
				e = nameMismatch
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
			e = notEndElement
			return
		}
		if name != "" && ee.Name.Local != name {
			log.Printf("required </%s>, found </%s>", name, ee.Name.Local)
			e = nameMismatch
			return
		}
	}
	// log.Printf("  .")
	return
}

func (st *state) getStart(name string) (se xml.StartElement, e error) {
	var t xml.Token
	t, _, e = st.token(tokStart, name)
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

func to_xml(v interface{}, typ bool) (s string) {
	r := reflect.ValueOf(v)
	t := r.Type()
	k := t.Kind()

	if b, ok := v.([]byte); ok {
		return "<base64>" + base64.StdEncoding.EncodeToString(b) + "</base64>"
	}

	switch k {
	case reflect.Invalid:
		panic("Unsupported type")
	case reflect.Bool:
		return fmt.Sprintf("<boolean>%v</boolean>", v)
	case reflect.Int,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if typ {
			return fmt.Sprintf("<int>%v</int>", v)
		}
		return fmt.Sprintf("%v", v)
	case reflect.Uintptr:
		panic("Unsupported type")
	case reflect.Float32, reflect.Float64:
		if typ {
			return fmt.Sprintf("<double>%v</double>", v)
		}
		return fmt.Sprintf("%v", v)
	case reflect.Complex64, reflect.Complex128:
		panic("Unsupported type")
	case reflect.Array, reflect.Slice:
		s = "<array><data>"
		for n := 0; n < r.Len(); n++ {
			s += "<value>"
			s += to_xml(r.Index(n).Interface(), typ)
			s += "</value>"
		}
		s += "</data></array>"
		return s
	case reflect.Chan:
		panic("Unsupported type")
	case reflect.Func:
		panic("Unsupported type")
	case reflect.Interface:
		return to_xml(r.Elem(), typ)
	case reflect.Map:
		s = "<struct>"
		for _, key := range r.MapKeys() {
			s += "<member>"
			s += "<name>" + xmlEscape(key.Interface().(string)) + "</name>"
			s += "<value>" + to_xml(r.MapIndex(key).Interface(), typ) + "</value>"
			s += "</member>"
		}
		return s + "</struct>"
	case reflect.Ptr:
		panic("Unsupported type")
	case reflect.String:
		if typ {
			return fmt.Sprintf("<string>%v</string>", xmlEscape(v.(string)))
		}
		return xmlEscape(v.(string))
	case reflect.Struct:
		s = "<struct>"
		for n := 0; n < r.NumField(); n++ {
			s += "<member>"
			s += "<name>" + t.Field(n).Name + "</name>"
			s += "<value>" + to_xml(r.FieldByIndex([]int{n}).Interface(), true) + "</value>"
			s += "</member>"
		}
		return s + "</struct>"
	case reflect.UnsafePointer:
		return to_xml(r.Elem(), typ)
	}
	return
}

func Call(url, name string, args ...interface{}) (interface{}, *Fault, error) {
	req := bytes.NewBuffer(nil)
	e := Marshal(req, name, args...)
	if e != nil {
		return nil, nil, e
	}
	r, e := http.Post(url, "text/xml", req)
	if e != nil {
		return nil, nil, e
	}
	defer r.Body.Close()

	_, v, f, e := Unmarshal(r.Body)
	return v, f, e
}

func WriteXml(w io.Writer, v interface{}, typ bool) (err error) {
	logger.SetPrefix("WriteXml")
	if b, ok := v.([]byte); ok {
		length := base64.StdEncoding.EncodedLen(len(b))
		dst := make([]byte, length)
		base64.StdEncoding.Encode(dst, b)
		_, err = taggedWrite(w, []byte("base64"), dst)
		return
	}
	if tim, ok := v.(time.Time); ok {
		_, err = taggedWriteString(w, "dateTime.iso8601", tim.Format(FullXmlRpcTime))
		return
	}
	r := reflect.ValueOf(v)
	t := r.Type()
	k := t.Kind()

	switch k {
	case reflect.Invalid:
		return Unsupported
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
	case reflect.Uintptr:
		return Unsupported
	case reflect.Float32, reflect.Float64:
		if typ {
			_, err = fmt.Fprintf(w, "<double>%v</double>", v)
			return err
		}
		_, err = fmt.Fprintf(w, "%v", v)
		return err
	case reflect.Complex64, reflect.Complex128:
		return Unsupported
	case reflect.Array, reflect.Slice:
		if _, err = io.WriteString(w, "<array><data>\n"); err != nil {
			return
		}
		n := r.Len()
		for i := 0; i < n; i++ {
			if _, err = io.WriteString(w, "  <value>"); err != nil {
				return
			}
			if err = WriteXml(w, r.Index(i).Interface(), typ); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value>\n"); err != nil {
				return
			}
		}
		if _, err = io.WriteString(w, "</data></array>\n"); err != nil {
			return
		}
	case reflect.Chan:
		return Unsupported
	case reflect.Func:
		return Unsupported
	case reflect.Interface:
		return WriteXml(w, r.Elem(), typ)
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
			if err = WriteXml(w, r.MapIndex(key).Interface(), typ); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value></member>\n"); err != nil {
				return
			}
		}
		_, err = io.WriteString(w, "</struct>")
		return
	case reflect.Ptr:
		return Unsupported
	case reflect.String:
		if typ {
			_, err = fmt.Fprintf(w, "<string>%v</string>", xmlEscape(v.(string)))
			return
		}
		_, err = io.WriteString(w, xmlEscape(v.(string)))
		return
	case reflect.Struct:
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
			if err = WriteXml(w, r.Field(i).Interface(), true); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value></member>"); err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, "</struct>")
		return
	case reflect.UnsafePointer:
		return WriteXml(w, r.Elem(), typ)
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

func Marshal(w io.Writer, name string, args ...interface{}) (err error) {
	if name == "" {
		if _, err = io.WriteString(w, "<methodResponse>"); err != nil {
			return
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
		if err = WriteXml(w, arg, true); err != nil {
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

func Unmarshal(r io.Reader) (name string, params []interface{}, fault *Fault, e error) {
	p := xml.NewDecoder(r)
	st := newParser(p)
	typ := "methodResponse"
	if _, e = st.getStart(typ); e == nameMismatch { // methodResponse or methodCall
		typ = "methodCall"
		if name, e = st.getText("methodName"); e != nil {
			return
		}
	}
	if _, e = st.getStart("params"); e != nil {
		return
	}
	params = make([]interface{}, 0, 8)
	var v interface{}
	for {
		if _, e = st.getStart("param"); e != nil {
			if e == notStartElement {
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
