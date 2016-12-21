package main

import (
	"bytes"
	"fmt"

	"reflect"

	//	"github.com/alecthomas/repr"

	"github.com/ugorji/go/codec"
	"github.com/yuin/gluamapper"
	"github.com/yuin/gopher-lua"
)

type Test struct {
	/*第一行是每个字段的注释*/
	Sid int
	/*第一行是每个字段的注释*/
	Int8 int8
	/*第一行是每个字段的注释*/
	Int16 int16
	/*第一行是每个字段的注释*/
	Int32 int32
	/*第一行是每个字段的注释*/
	Int64 int64
	/*第一行是每个字段的注释*/
	Int int
	/*第一行是每个字段的注释*/
	Float32 float32
	/*第一行是每个字段的注释*/
	Float64 float64
	/*第一行是每个字段的注释*/
	String string
	/*第一行是每个字段的注释*/
	Bool bool
	/*第一行是每个字段的注释*/
	Int8s []int8
	/*第一行是每个字段的注释*/
	Int16s []int16
	/*第一行是每个字段的注释*/
	Int32s []int32
	/*第一行是每个字段的注释*/
	Int64s []int64
	/*第一行是每个字段的注释*/
	Ints []int
	/*第一行是每个字段的注释*/
	Float32s []float32
	/*第一行是每个字段的注释*/
	Float64s []float64
	/*第一行是每个字段的注释*/
	Strings []string
	/*第一行是每个字段的注释*/
	Bools []bool
	/*第一行是每个字段的注释*/
	Map_Int_Int map[int]int
	/*第一行是每个字段的注释*/
	Map_Int_String map[int]string
	/*第一行是每个字段的注释*/
	Map_Int_Strings map[int][]string
	/*第一行是每个字段的注释*/
	Map_String_Ints map[string][]int
	/*第一行是每个字段的注释*/
	Map_String_Strings map[string][]string
	/*第一行是每个字段的注释*/
	Struct Struct
	/*第一行是每个字段的注释*/
	Structs []Struct
	/*第一行是每个字段的注释*/
	Structss [][]Struct
	/*第一行是每个字段的注释*/
	Map_String_Struct map[string]Struct
	/*第一行是每个字段的注释*/
	Map_Int_Struct map[int]Struct
	/*第一行是每个字段的注释*/
	Map_Int_Structs map[int][]Struct
	/*第一行是每个字段的注释*/
	Map_String_Structs map[string][]Struct
	/*第一行是每个字段的注释*/
	Map_String_Structss map[string][][]Struct
}
type Struct struct {
	AA int `lua:"aa",codec:"aa"`
	BB int `lua:"bb",codec:"bb"`
	CC Struct1
}
type Struct1 struct {
	CC int `lua:"cc",codec:"cc"`
	DD int `lua:"dd",codec:"dd"`
}

func main() {
	L := lua.NewState()
	test := new(Test)
	mapper := gluamapper.NewMapper(gluamapper.Option{NameFunc: gluamapper.Id})
	if err := L.DoFile("./test.lua"); err != nil {
		panic(err)
	}
	if err := mapper.Map(L.GetGlobal("test").(*lua.LTable), &test); err != nil {
		panic(err)
	}
	//	fmt.Printf("test=%+v\n", repr.Repr(test, repr.Indent(" ")))
	var mh codec.MsgpackHandle
	var w bytes.Buffer
	if err := codec.NewEncoder(&w, &mh).Encode(test); err != nil {
		panic(err)
	}
	//	fmt.Printf("%v\n", w.Bytes())
	r := bytes.NewReader(w.Bytes())
	test1 := new(Test)
	if err := codec.NewDecoder(r, &mh).Decode(test1); err != nil {
		panic(err)
	}

	//	fmt.Printf("test1=%+v\n", repr.Repr(test1, repr.Indent(" ")))
	fmt.Println(reflect.DeepEqual(test1, test))
}
