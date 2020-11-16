package util

import (
	"fmt"
	"reflect"
	"strings"
)

//解析 路径
func Parse(url string, data interface{}) error {
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Ptr && val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("url kind not  ptr")
	}
	typ := reflect.TypeOf(data)
	paths := strings.Split(url, "/")
	m := toMap(paths)
	setValue(val, typ, m)
	return nil
}

func setValue(val reflect.Value, typ reflect.Type, m map[string]interface{}) {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}
	fields := val.NumField()
	for i := 0; i < fields; i++ {
		fval := val.Field(i)
		ftyp := typ.Field(i)
		switch fval.Kind() {
		case reflect.String:
			tag := ftyp.Tag.Get("json")
			value := fmt.Sprintf("%v", m[tag])
			fval.SetString(value)
		case reflect.Slice:
			tag := ftyp.Tag.Get("json")
			value := fmt.Sprintf("%v", m[tag])
			fval.Set(reflect.ValueOf(strings.Split(value, ",")))
		case reflect.Ptr:
			if fval.IsNil() {
				continue
			}
			setValue(fval, ftyp.Type, m)
		case reflect.Struct:
			setValue(fval, ftyp.Type, m)
		default:
			continue
		}
	}
}

func toMap(paths []string) map[string]interface{} {
	m := make(map[string]interface{}, 0)
	l := len(paths)
	for i := 0; ; {
		if i >= l {
			break
		}
		key := paths[i]
		i++
		if i >= l {
			break
		}
		value := paths[i]
		i++
		m[key] = value
	}
	return m
}
