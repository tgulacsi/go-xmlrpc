package xmlrpc

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"
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
      <param><value><string>árvízűtő tükörfúrógép</string></value></param>
      <param><value><double>-0.333333</double></value></param>
      <param><value><base64>eW91IGNhbid0IHJlYWQgdGhpcyE=</base64></value></param>
      <param><value><string>!last param!</string></value></param>
      </params>
   </methodResponse>`

const XmlFault = `<?xml version="1.0"?>
<methodResponse>
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
	time.Now(),
	[]byte{1, 2, 3, 5, 7, 11, 13, 17},
	[]interface{}{"a", "b"},
	map[string]string{"k": "v"},
}

func TestResponse(t *testing.T) {
	name, c, fault, err := Unmarshal(bytes.NewBufferString(XmlCallString))
	if err != nil {
		t.Fatal("error unmarshaling XmlCall:", err)
	}
	fmt.Printf("unmarshalled call[%s]: %=v\n", name, c)

	name, c, fault, err = Unmarshal(bytes.NewBufferString(XmlResponse))
	if err != nil {
		t.Fatal("error unmarshaling XmlResponse:", err)
	}
	fmt.Printf("unmarshalled response[%s]: %=v\n%s\n", name, c, fault)

	buf := bytes.NewBuffer(nil)
	err = Marshal(buf, "trial", XmlCallStruct)
	if err != nil {
		t.Fatal("error marshalling XmlCallStruct:", err)
	}
	fmt.Printf("marshalled %=v\n:\n%s\n", XmlCallStruct, buf.Bytes())

	name, c, fault, err = Unmarshal(buf)
	if err != nil {
		t.Fatal("cannot unmarshal previously marshalled struct:", err)
	}
	if name != "trial" {
		t.Error("name mismatch")
	}
	if !reflect.DeepEqual(c, XmlCallStruct) {
		t.Errorf("struct mistmatch:\n%=v\n!=\n%=v\n", c, XmlCallStruct)
	}
}
