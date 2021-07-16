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
	K33  int      `json:"k33"`
	Km1  []string `json:"km1"`
	Foo  *Foo     `json:"foo"`
	Foo1 Foo      `json:"foo1"`
}

type Foo struct {
	Km2 string   `json:"km2"`
	Km3 []string `json:"km3"`
}

type TestData2 struct {
	Km        string   `json:"km" default:"kkxx"`
	K33       int      `json:"k33"`
	Km1       []string `json:"km1"`
	Foo       *Foo     `json:"foo"`
	Foo1      Foo      `json:"foo1"`
	Duration  bool     `json:"duration"  default:"true"`
	Duration1 bool     `json:"duration1"  default:"false"`
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
	err := setValue(reflect.ValueOf(td).Elem(), reflect.TypeOf(td).Elem(), m)
	assert.Equal(t, nil, err)
	fmt.Println("ss:", td)
	assert.Equal(t, foo, td.Foo)
	td = &TestData1{Foo: &Foo{}}
	err = setValue(reflect.ValueOf(td).Elem(), reflect.TypeOf(td).Elem(), m)
	assert.Equal(t, Foo{
		Km2: "vm12,vv,xx",
		Km3: []string{"vm", "vv123", "xx22"},
	}, *td.Foo)
	assert.Equal(t, nil, err)

}

func TestParse(t *testing.T) {
	d := TestData1{}
	err := Parse("k/c/km/kv,xx/k33/13", d)
	assert.Equal(t, nil, err)
	assert.Equal(t, "", d.Km)
	assert.Equal(t, 0, d.K33)
	td := &TestData1{}
	err = Parse("k/c/km/kv,xx", td)
	assert.Equal(t, "kv,xx", td.Km)
	assert.Equal(t, nil, err)

	err = Parse("k/c/km/kv,xx/k33/13", td)
	assert.Equal(t, nil, err)
	assert.Equal(t, "kv,xx", td.Km)
	assert.Equal(t, 13, td.K33)

	err = Parse("k/c/km/kv,xx/k33/ww", td)
	assert.Equal(t, "strconv.ParseInt: parsing \"ww\": invalid syntax", err.Error())

	err = Parse("/", td)
	assert.Equal(t, err, nil)

	err = Parse("/k/c/km/kv,xx/k33/13", td)
	assert.Equal(t, nil, err)
	assert.Equal(t, "kv,xx", td.Km)
	assert.Equal(t, 13, td.K33)

	tdd := &TestData2{}
	err = Parse("/k/c/km//k33/13", tdd)
	assert.Equal(t, nil, err)
	assert.Equal(t, "kkxx", tdd.Km)
}

func TestParse2(t *testing.T) {

	tdd := &TestData2{}
	err := Parse("/k/c/km//k33/13", tdd)
	assert.Equal(t, nil, err)
	assert.Equal(t, "kkxx", tdd.Km)
	tdd = &TestData2{}
	err = Parse("/k/c/km//k33/13/duration//duration1/true", tdd)
	assert.Equal(t, nil, err)
	assert.Equal(t, "kkxx", tdd.Km)
	assert.Equal(t, true, tdd.Duration)
	assert.Equal(t, true, tdd.Duration1)

	err = Parse("/k/c/km//k33/13/duration/asdasd/duration1/true", tdd)
	assert.Equal(t, "strconv.ParseBool: parsing \"asdasd\": invalid syntax", err.Error())
}
