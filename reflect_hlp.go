package xmlrpc

import (
	"fmt"
	"log"
	"reflect"
	"strings"
)

func FillStruct(dst interface{}, src interface{}) (err error) {
	log.Printf("FillStruct(%+v, %+v)", dst, src)
	t := reflect.TypeOf(dst)
	k := t.Kind()
	for k == reflect.Ptr {
		dst2r := reflect.ValueOf(dst).Elem()
		k = dst2r.Kind()
		if k == reflect.Struct || k != reflect.Ptr {
			break
		}
		log.Printf("indirected %+v to %+v", dst, dst2r.Interface())
		dst = dst2r.Interface()
	}
	if amap, ok := src.(map[string]interface{}); ok {
		if k == reflect.Struct {
			return fillStructWithMap(dst, amap)
		}
	}
	if alist, ok := src.([]interface{}); ok {
		log.Printf("len(src)=%d", len(alist))
		if len(alist) == 1 {
			log.Printf("k=%s", k)
			if k == reflect.Struct {
				if amap, ok := alist[0].(map[string]interface{}); ok {
					return fillStructWithMap(dst, amap)
				}
			} else {
				v := reflect.ValueOf(dst)
				if v.Type().Kind() == reflect.Ptr {
					log.Printf("dst kind=%s", v.Kind())
					v.Elem().Set(reflect.ValueOf(alist[0]))
					return nil
				}
				if ptr, ok := dst.(*interface{}); ok {
					*ptr = alist[0]
					return nil
				}
				return fmt.Errorf("cannot push %+v into %+v (list len=1 of %T into not struct %T)", src, dst, alist[0], dst)
			}

		}
		return fillStructWithSlice(dst, alist)
	}
	return fmt.Errorf("unsupported type %T (%+v)", src, src)
}

func fillStructWithMap(dst interface{}, src map[string]interface{}) error {
	log.Printf("fillStructWithMap(%+v, %+v)", dst, src)
	var (
		ok bool
		sv reflect.Value
	)
	t := reflect.TypeOf(dst)
	v := reflect.ValueOf(dst)
	if t.Kind() == reflect.Ptr {
		v = reflect.Indirect(v)
		t = v.Type()
	}
	log.Printf(" t=%s k=%s", t, t.Kind())
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

func fillStructWithSlice(dst interface{}, src []interface{}) error {
	log.Printf("fillStructWithSlice(%+v, %+v)", dst, src)
	v := reflect.ValueOf(dst)
	t := v.Type()
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
