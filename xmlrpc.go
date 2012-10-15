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
	last      xml.StartElement
}

func newParser(p *xml.Decoder) *state {
	return &state{p, 0, nil, xml.StartElement{}}
}

func (st *state) next() (nm xml.Name, nv interface{}, e error) {
	if st.remainder != nil {
		nv = *st.remainder
		st.remainder = nil
		return
	}
	if st.level < 0 {
		panic("st.level < 0")
	}
	var se xml.StartElement
	for {
		se, e = st.nextStart()
		if e == nil {
			if se.Name.Local == "param" {
				e = levelDecremented
				return
			}
			break
		} else if e != nil && e != levelDecremented {
			return
		}
	}
	log.Print("next level:", st.level, " meets <", se.Name.Local, ">")

	var vn valueNode
	switch se.Name.Local {
	case "boolean":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		b, e := strconv.ParseBool(vn.Body)
		return xml.Name{}, b, e
	case "string":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		return xml.Name{}, vn.Body, nil
	case "int", "i4":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		i, e := strconv.ParseInt(strings.TrimSpace(vn.Body), 10, 64)
		return xml.Name{}, i, e
	case "double":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		f, e := strconv.ParseFloat(strings.TrimSpace(vn.Body), 64)
		return xml.Name{}, f, e
	case "dateTime.iso8601":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		var t time.Time
		for _, format := range []string{FullXmlRpcTime, LocalXmlRpcTime, DenseXmlRpcTime} {
			t, e = time.Parse(format, vn.Body)
			// log.Print("txt=", vn.Body, " t=", t, " fmt=", format, " e=", e)
			if e == nil {
				break
			}
		}
		return xml.Name{}, t, e
	case "base64":
		if e = st.p.DecodeElement(&vn, &se); e != nil {
			return
		}
		var b []byte
		if b, e = base64.StdEncoding.DecodeString(vn.Body); e != nil {
			return
		} else {
			return xml.Name{}, b, nil
		}
	case "member":
		if _, e = st.nextStart(); e != nil {
			return
		}
		return st.next()
	case "value":
		return st.next()
	case "name":
		if _, e = st.nextStart(); e != nil {
			return
		}
		return st.next()
	case "struct":
		st.level++ // Entering new struct level. Increase global level.
		// localLevel := st.level // And set local to current.

		strct := make(map[string]interface{}, 8)

		if se, e = st.nextStart(); e != nil {
			log.Println("struct first step error:", e)
			return
		}
		var value interface{}
		var name string
		for e == nil && se.Name.Local == "member" {
			// name
			if se, e = st.nextStart(); e != nil {
				return
			}
			if se.Name.Local != "name" {
				e = fmt.Errorf("invalid response: start element is '%s', not 'name'", se.Name.Local)
				return
			}
			if e = st.p.DecodeElement(&vn, &se); e != nil {
				return
			}
			name = vn.Body
			// log.Print("  struct name=", name)
			if se, e = st.nextStart(); e != nil {
				return
			}

			// value
			if _, value, e = st.next(); e != nil {
				return
			}
			if se.Name.Local != "value" {
				e = fmt.Errorf("invalid response: found '%s', required 'value'", se.Name.Local)
				return
			}

			strct[name] = value
			se, e = st.nextStart()
			// log.Print("last se=", se.Name.Local)
		}
		// log.Print("struct returns with sl=", st.level, " strct=", strct, " e=", e)
		if e == levelDecremented {
			e = nil
		}
		return xml.Name{}, strct, e
	case "array":
		st.level++ // Entering new struct level. Increase global level.
		// localLevel := st.level // And set local to current.

		// data
		if se, e = st.nextStart(); e != nil {
			log.Println("array first step returns with ", e)
			return
		}
		if se.Name.Local != "data" {
			e = fmt.Errorf("found '%s' instead of 'data'", se.Name.Local)
			return
		}
		ar := make([]interface{}, 0, 8)
		var value interface{}
		for {
			_, value, e = st.next()
			// log.Print("array se=", se.Name.Local, " sl=", st.level,
			// 	" val=", value, " e=", e)
			if e != nil {
				if e == io.EOF {
					if value != nil {
						ar = append(ar, value)
					}
					break
				}
				return
			}
			ar = append(ar, value)
		}
		if e == levelDecremented {
			e = nil
		}
		// log.Print("array returns ", ar)
		return xml.Name{}, ar, e
	}

	if e = st.p.DecodeElement(nv, &se); e != nil {
		return
	}
	return se.Name, nv, e
}

// jumps to the next start element, returns it
func (st *state) nextStart() (xml.StartElement, error) {
	if st.level < 0 {
		panic("st.level < 0")
	}
	for {
		t, e := st.p.Token()
		if e != nil {
			if e != io.EOF {
				log.Printf("error going to next token:", e)
			}
			return xml.StartElement{}, e
		}
		switch t := t.(type) {
		case xml.StartElement:
			st.last = t
			return t, nil
		case xml.EndElement:
			// log.Print("nextStart jumps </", t.Name.Local, ">", " level=", st.level)
			if t.Name.Local == "struct" || t.Name.Local == "array" { // Found struct end. Decrease struct level.
				st.level--
				return xml.StartElement{}, levelDecremented
				// if st.level < 0 {
				// 	panic("st.level < 0")
				// }
			}
		}
	}
	panic("unreachable")
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
	se, e := st.nextStart() // methodResponse or methodCall
	if se.Name.Local != "methodResponse" {
		se, e = st.nextStart() // methodName
		if se.Name.Local != "methodName" {
			e = fmt.Errorf("invalid call: requred 'methodName', found '%s'", se.Name.Local)
			return
		}
		t, err := p.Token()
		if err != nil {
			e = err
			return
		}
		d, ok := t.(xml.CharData)
		if !ok {
			e = fmt.Errorf("invalid call: required call name, found '%v'", t)
			return
		}
		name = string(d.Copy())
	}
	se, e = st.nextStart() // params
	if se.Name.Local != "params" {
		e = fmt.Errorf("invalid response: required 'params', found '%s'", se.Name.Local)
		return
	}
	params = make([]interface{}, 0, 8)
	var v interface{}
	// param
	if se, e = st.nextStart(); e != nil {
		if e == io.EOF {
			e = nil
		}
		return
	}
	if se.Name.Local != "param" {
		e = fmt.Errorf("invalid response: required 'param', found '%s'", se.Name.Local)
		return
	}
	for {
		if st.last.Name.Local != "param" {
			if se, e = st.nextStart(); e != nil {
				if e == io.EOF {
					e = nil
				}
				return
			}

		}
		if _, v, e = st.next(); e != nil {
			log.Println("nxt=", e, " v=", v)
			if e != levelDecremented {
				if e == io.EOF {
					e = nil
					if v != nil {
						params = append(params, v)
					}
					break
				}
				return
			}
		}
		params = append(params, v)
	}
	return
}
