package util

// Parsing the request path encapsulates the parsed value into the object
import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

//resolution path
func Parse(url string, data interface{}) (err error) {
	match, _ := regexp.MatchString("^/.*", url)
	if match {
		if len(url) > 1 {
			url = url[1:]
		} else {
			return
		}
	}
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return
	}
	typ := reflect.TypeOf(data)
	paths := strings.Split(url, "/")
	m := toMap(paths)
	return setValue(val, typ, m)
}

const (
	Tag        = "json"
	DefaultTag = "default"
)

func setValue(val reflect.Value, typ reflect.Type, m map[string]interface{}) error {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}
	fields := val.NumField()
	for i := 0; i < fields; i++ {
		fval := val.Field(i)
		ftyp := typ.Field(i)
		tag := ftyp.Tag.Get(Tag)
		defaultValue := ftyp.Tag.Get(DefaultTag)
		value := m[tag]
		switch fval.Kind() {
		case reflect.String:
			va := ""
			if m[tag] == nil || fmt.Sprintf("%v", value) == "" {
				va = defaultValue
			} else {
				va = fmt.Sprintf("%v", value)
			}
			fval.SetString(va)
		case reflect.Int:
			var va int64
			//
			if value != nil {
				value, err := strconv.ParseInt(fmt.Sprintf("%v", value), 10, 64)
				va = value
				if err != nil {
					return err
				}
			} else {
				value, _ := strconv.ParseInt(fmt.Sprintf("%v", defaultValue), 10, 64)
				va = value
			}
			fval.SetInt(va)
		case reflect.Bool:
			var va bool
			//
			if value != nil && value != "" {
				value, err := strconv.ParseBool(fmt.Sprintf("%v", value))
				va = value
				if err != nil {
					return err
				}
			} else {
				value, _ := strconv.ParseBool(fmt.Sprintf("%v", defaultValue))
				va = value
			}
			fval.SetBool(va)
		case reflect.Slice:
			va := make([]string, 0)
			if value != nil && value != "" {
				va = strings.Split(fmt.Sprintf("%v", value), ",")

			}
			fval.Set(reflect.ValueOf(va))
		case reflect.Ptr:
			if fval.IsNil() {
				continue
			}
			err := setValue(fval, ftyp.Type, m)
			if err != nil {
				return err
			}
			/*		case reflect.Struct:
					err := setValue(fval, ftyp.Type, m)
					if err != nil {
						return err
					}*/
			/*default:
			continue*/
		}
	}
	return nil
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

//valid 对path 进行校验
func valid() {

}
