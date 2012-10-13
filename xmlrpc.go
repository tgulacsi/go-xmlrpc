package xmlrpc

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var unsupported = errors.New("unsupported type")

type Array []interface{}
type Struct map[string]interface{}
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

func next(p *xml.Decoder, structLevel int) (nm xml.Name, structLeve int, nv interface{}, e error) {
	var se xml.StartElement
	if se, structLevel, e = nextStart(p, structLevel); e != nil {
		return
	}

	var vn valueNode
	switch se.Name.Local {
	case "boolean":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		b, e := strconv.ParseBool(vn.Body)
		return xml.Name{}, structLevel, b, e
	case "string":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		return xml.Name{}, structLevel, vn.Body, nil
	case "int", "i1", "i2", "i4", "i8":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		i, e := strconv.ParseInt(strings.TrimSpace(vn.Body), 10, 64)
		return xml.Name{}, structLevel, i, e
	case "double":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		f, e := strconv.ParseFloat(strings.TrimSpace(vn.Body), 64)
		return xml.Name{}, structLevel, f, e
	case "dateTime.iso8601":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		t, e := time.Parse("20060102T15:04:05", vn.Body)
		if e != nil {
			t, e = time.Parse("2006-01-02T15:04:05-07:00", vn.Body)
			if e != nil {
				t, e = time.Parse("2006-01-02T15:04:05", vn.Body)
			}
		}
		return xml.Name{}, structLevel, t, e
	case "base64":
		if e = p.DecodeElement(&vn, &se); e != nil {
			return
		}
		var b []byte
		if b, e = base64.StdEncoding.DecodeString(vn.Body); e != nil {
			return
		} else {
			return xml.Name{}, structLevel, b, nil
		}
	case "member":
		if _, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		return next(p, structLevel)
	case "value":
		if _, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		return next(p, structLevel)
	case "name":
		if _, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		return next(p, structLevel)
	case "struct":
		structLevel++             // Entering new struct level. Increase global level.
		localLevel := structLevel // And set local to current.

		st := Struct{}

		if se, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		var value interface{}
		var name string
		for e == nil && se.Name.Local == "member" {
			// name
			if se, structLevel, e = nextStart(p, structLevel); e != nil {
				return
			}
			if se.Name.Local != "name" {
				e = errors.New("invalid response")
				return
			}
			if e = p.DecodeElement(&vn, &se); e != nil {
				return
			}
			name = vn.Body
			if se, structLevel, e = nextStart(p, structLevel); e != nil {
				return
			}

			// value
			if _, structLevel, value, e = next(p, structLevel); e != nil {
				return
			}
			if se.Name.Local != "value" {
				e = errors.New("invalid response")
				return
			}
			st[name] = value

			if localLevel > structLevel { // We came up from higher level. We're already on a Start.
				break
			}
			if se, structLevel, e = nextStart(p, structLevel); e != nil {
				return
			}
		}
		return xml.Name{}, structLevel, st, nil
	case "array":
		var ar Array
		// data
		if _, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		// top of value
		if _, structLevel, e = nextStart(p, structLevel); e != nil {
			return
		}
		var value interface{}
		for {
			if _, structLevel, value, e = next(p, structLevel); e != nil {
				break
			}
			ar = append(ar, value)
		}
		return xml.Name{}, structLevel, ar, nil
	}

	if e = p.DecodeElement(nv, &se); e != nil {
		return
	}
	return se.Name, structLevel, nv, e
}

// jumps to the next start element, returns it
func nextStart(p *xml.Decoder, sl int) (xml.StartElement, int, error) {
	for {
		t, e := p.Token()
		if e != nil {
			return xml.StartElement{}, sl, e
		}
		switch t := t.(type) {
		case xml.StartElement:
			return t, sl, nil
		case xml.EndElement:
			if t.Name.Local == "struct" { // Found struct end. Decrease struct level.
				sl--
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
		panic("unsupported type")
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
		panic("unsupported type")
	case reflect.Float32, reflect.Float64:
		if typ {
			return fmt.Sprintf("<double>%v</double>", v)
		}
		return fmt.Sprintf("%v", v)
	case reflect.Complex64, reflect.Complex128:
		panic("unsupported type")
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
		panic("unsupported type")
	case reflect.Func:
		panic("unsupported type")
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
		panic("unsupported type")
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
	s := "<methodCall>"
	s += "<methodName>" + xmlEscape(name) + "</methodName>"
	s += "<params>"
	for _, arg := range args {
		s += "<param><value>"
		s += to_xml(arg, true) // Warning changed typed arguments to TRUE !
		s += "</value></param>"
	}
	s += "</params></methodCall>"
	bs := bytes.NewBuffer([]byte(s))
	r, e := http.Post(url, "text/xml", bs)
	if e != nil {
		return nil, nil, e
	}
	defer r.Body.Close()

	return Decode(r.Body)
}

func WriteXml(w io.Writer, v interface{}, typ bool) (err error) {
	r := reflect.ValueOf(v)
	t := r.Type()
	k := t.Kind()

	if b, ok := v.([]byte); ok {
		dst := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
		base64.StdEncoding.Encode(dst, b)
		_, err = w.Write(b)
		return
	}

	switch k {
	case reflect.Invalid:
		return unsupported
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
		return unsupported
	case reflect.Float32, reflect.Float64:
		if typ {
			_, err = fmt.Fprintf(w, "<double>%v</double>", v)
			return err
		}
		_, err = fmt.Fprintf(w, "%v", v)
		return err
	case reflect.Complex64, reflect.Complex128:
		return unsupported
	case reflect.Array, reflect.Slice:
		if _, err = io.WriteString(w, "<array><data>"); err != nil {
			return
		}
		n := r.Len()
		for i := 0; i < n; i++ {
			if _, err = io.WriteString(w, "<value>"); err != nil {
				return
			}
			if err = WriteXml(w, r.Index(i).Interface(), typ); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</value>"); err != nil {
				return
			}
		}
		if _, err = io.WriteString(w, "</data></array>"); err != nil {
			return
		}
	case reflect.Chan:
		return unsupported
	case reflect.Func:
		return unsupported
	case reflect.Interface:
		return WriteXml(w, r.Elem(), typ)
	case reflect.Map:
		if _, err = io.WriteString(w, "<struct>"); err != nil {
			return
		}
		for _, key := range r.MapKeys() {
			if _, err = io.WriteString(w, "<member><name>"); err != nil {
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
			if _, err = io.WriteString(w, "</value></member>"); err != nil {
				return
			}
		}
		_, err = io.WriteString(w, "</struct")
		return
	case reflect.Ptr:
		return unsupported
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
			if _, err = io.WriteString(w, "<member><name>"); err != nil {
				return
			}
			if _, err = io.WriteString(w, xmlEscape(t.Field(i).Name)); err != nil {
				return
			}
			if _, err = io.WriteString(w, "</name><value>"); err != nil {
				return
			}
			if err = WriteXml(w, r.FieldByIndex([]int{i}).Interface(), true); err != nil {
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

func taggedWrite(w io.Writer, tag, inner string) (n int, err error) {
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
		if _, err = io.WriteString(w, "</methodName>"); err != nil {
			return
		}
	}
	if _, err = io.WriteString(w, "<params"); err != nil {
		return
	}
	for _, arg := range args {
		if _, err = io.WriteString(w, "<param><value>"); err != nil {
			return
		}
		if err = WriteXml(w, arg, true); err != nil {
			return
		}
		if _, err = io.WriteString(w, "</value></param>"); err != nil {
			return
		}
	}
	_, err = io.WriteString(w, "</params></methodCall>")
	return err
}

func Decode(r io.Reader) (params []interface{}, fault *Fault, e error) {
	p := xml.NewDecoder(r)
	structLevel := 0
	se, structLevel, e := nextStart(p, structLevel) // methodResponse
	if se.Name.Local != "methodResponse" {
		return nil, nil, errors.New("invalid response")
	}
	se, structLevel, e = nextStart(p, structLevel) // params
	if se.Name.Local != "params" {
		return nil, nil, errors.New("invalid response")
	}
	var v interface{}
	for {
		// param
		if se, structLevel, e = nextStart(p, structLevel); e != nil {
			if e == io.EOF {
				e = nil
				break
			}
			return
		}
		if se.Name.Local != "param" {
			return nil, nil, errors.New("invalid response")
		}
		// value
		if se, structLevel, e = nextStart(p, structLevel); e != nil {
			if e == io.EOF {
				e = nil
				break
			}
			return
		}
		if se.Name.Local != "value" {
			return nil, nil, errors.New("invalid response")
		}
		if _, structLevel, v, e = next(p, structLevel); e != nil {
			if e == io.EOF {
				e = nil
				break
			}
			return
		}
		params = append(params, v)
	}
	return
}
