package xmlrpc

import (
	"bytes"
	"fmt"
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
	"árvíztűrő tükörfúrógép", -0.333333, time.Now(), []byte("proba"),
}

func TestResponse(t *testing.T) {
	c, fault, err := Decode(bytes.NewBufferString(XmlResponse))
	if err != nil {
		t.Fatal("error unmarshaling XmlCallString:", err)
	}
	fmt.Printf("unmarshalled response: %+v\n%s", c, fault)
}
