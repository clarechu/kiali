package util

import (
	"fmt"
	"github.com/magiconair/properties/assert"
	"reflect"
	"testing"
)

func TestToMap(t *testing.T) {
	paths := []string{"namespace", "a"}
	m := toMap(paths)
	assert.Equal(t, "a", m["namespace"])
	paths = []string{}
	m = toMap(paths)
	assert.Equal(t, 0, len(m))
	paths = []string{"namespace", "a", "k"}
	m = toMap(paths)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, "a", m["namespace"])
	assert.Equal(t, nil, m["k"])
}

type TestData1 struct {
	Km   string   `json:"km"`
	Km1  []string `json:"km1"`
	Foo  *Foo     `json:"foo"`
	Foo1 Foo      `json:"foo1"`
}

type Foo struct {
	Km2 string   `json:"km2"`
	Km3 []string `json:"km3"`
}

func TestSet(t *testing.T) {
	m := map[string]interface{}{
		"km":  "vm",
		"km1": "vm,vv,xx",
		"km2": "vm12,vv,xx",
		"km3": "vm,vv123,xx22",
	}
	td := &TestData1{}
	foo := td.Foo
	setValue(reflect.ValueOf(td).Elem(), reflect.TypeOf(td).Elem(), m)
	fmt.Println("ss:", td)
	assert.Equal(t, foo, td.Foo)
	td = &TestData1{Foo: &Foo{}}
	setValue(reflect.ValueOf(td).Elem(), reflect.TypeOf(td).Elem(), m)
	assert.Equal(t, Foo{
		Km2: "vm12,vv,xx",
		Km3: []string{"vm", "vv123", "xx22"},
	}, *td.Foo)
}
