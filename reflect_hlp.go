package xmlrpc

import (
	"fmt"
	"log"
	"reflect"
	"strings"
)

func FillStruct(dst *interface{}, v interface{}) (err error) {
	t := reflect.TypeOf(*dst)
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("needs struct to fill, not %+v", *dst)
	}
	if amap, ok := v.(map[string]interface{}); ok {
		return fillStructWithMap(dst, amap)
	}
	if alist, ok := v.([]interface{}); ok {
		return fillStructWithSlice(dst, alist)
	}
	return fmt.Errorf("unsupported type %T (%+v)", v, v)
}

func fillStructWithMap(dst *interface{}, src map[string]interface{}) error {
	var (
		ok bool
		sv reflect.Value
	)
	t := reflect.TypeOf(*dst)
	v := reflect.ValueOf(*dst)
	for key, val := range src {
		if _, ok = t.FieldByName(key); !ok {
			key = strings.Title(key)
			if _, ok = t.FieldByName(key); !ok {
				log.Printf("unknown key %s", key)
				continue
			}
		}
		sv = v.FieldByName(key)
		if !sv.CanSet() {
			log.Printf("cannot set value of %#v (%s)", sv, key)
		}
		sv.Set(reflect.ValueOf(val))
	}
	return nil
}

func fillStructWithSlice(dst *interface{}, src []interface{}) error {
	t := reflect.TypeOf(*dst)
	v := reflect.ValueOf(*dst)
	n := t.NumField()
	var sv reflect.Value
	for i, val := range src {
		if i >= n {
			break
		}
		sv = v.Field(i)
		if !sv.CanSet() {
			log.Printf("cannot set value of %#v (%d)", sv, i)
		}
		sv.Set(reflect.ValueOf(val))
	}
	return nil
}
