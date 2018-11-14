// Copyright 2016 zxfonline@sina.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"os/exec"
	"unicode/utf8"

	"regexp"

	"text/template"

	//	"github.com/alecthomas/repr"

	"github.com/tealeg/xlsx"
	"github.com/zxfonline/strutil"

	"bytes"

	"sync"

	"github.com/jessevdk/go-flags"
)

type ExcelsOption struct {
	List map[string][]string
}

var (
	excelOptionValueReg    = regexp.MustCompile(`^[^\=\[\]]{0,}[\=]{1}\s{0,}\[[^\[\]]{0,}\][^\=\[\]]{0,}(\,[^\=\[\]]{0,}[\=]{1}\s{0,}\[[^\[\]]{0,}\][^\=\[\]]{0,}){0,}$`)
	excelOptionSonValueReg = regexp.MustCompile(`[^\,\=\[\]]{0,}[\=]{1}\s{0,}\[[^\[\]]{0,}\]`)
)

func (p *ExcelsOption) UnmarshalFlag(value string) error {
	p.List = make(map[string][]string, 0)
	if excelOptionValueReg.MatchString(value) {
		kvsm := excelOptionSonValueReg.FindAllString(value, -1)
		result := make(map[string]bool)
		for _, kvs := range kvsm {
			kv := strings.Split(kvs, "=")
			if len(kv) != 2 {
				return fmt.Errorf("invalid type value,err:format error.")
			}
			pathfile := strings.TrimSpace(kv[0])
			pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
			value := strings.TrimSpace(kv[1])
			value = value[1 : len(value)-1]
			sheetNames := strings.Split(value, ",")
			for _, sheetName := range sheetNames {
				if ex := result[sheetName]; ex {
					return fmt.Errorf("duplicate sheet:%+v", sheetName)
				}
				result[sheetName] = true
			}
			if list, pre := p.List[pathfile]; pre {
				list = append(list, sheetNames...)
			} else {
				list = make([]string, 0)
				list = append(list, sheetNames...)
				p.List[pathfile] = list
			}
		}
	} else {
		return fmt.Errorf("invalid type value,err:format error.")
	}
	return nil
}

func (p ExcelsOption) MarshalFlag() (string, error) {
	strs := make([]string, 0, len(p.List))
	for k, v := range p.List {
		strs = append(strs, fmt.Sprintf("%s=[%s]", k, strings.Join(v, ",")))
	}
	return strings.Join(strs, ","), nil
}

type Options struct {
	OutGoPath        string       `short:"g" long:"dgo" description:"golang 源文件输出目录"`
	OutluaPath       string       `short:"l" long:"dlua" description:"lua 源文件输出目录"`
	MapSeparator     string       `short:"m" long:"map_sep" default:"=" description:"map key=value 分隔符 默认 = "`
	ArraySeparator   string       `short:"a" long:"array_sep" default:"," description:"数组内容 分隔符 默认 , "`
	ArraysTokenBegin string       `short:"b" long:"token_begin" default:"[" description:"二维数组节点开始标记 默认 [ "`
	ArraysTokenEnd   string       `short:"e" long:"token_end" default:"]" description:"二维数组节点开始标记 默认 ] "`
	Indent           string       `short:"i" long:"indent" default:"\t" description:"节点排版间隔 默认 \t "`
	Excels           ExcelsOption `short:"f" long:"excels" description:"Excel导出文件 格式:file1=[sheet1,sheet2,...],file2=[sheet1,...],..."`
}

var (
	//map key=value 默认分隔符
	MAP_SEPARATOR = "="

	//数组 [1,2,3] 默认分隔符
	ARRAY_SEPARATOR = ","

	//二维数组节点开始标记
	ARRAYS_TOKEN_BEGIN = "["
	//二维数组节点结束标记
	ARRAYS_TOKEN_END = "]"

	INDENT = "\t"

	MAINKEY_INDEX = 0
)

var (
	//golang 数据类型处理

	//基础数据类型(int8、int16、int32、int64、int、float32、float64、string、bool) 和 对应的数值(一维、二维)eg:int、[]int、[][]int
	baseReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}$`)
	//map[key]value key=基础数据类型 value=基础数据类型 、 基础数据类型 一维数组 、 基础数据类型 二维数组 eg:map[int]int、map[int][]int、map[int][][]int
	baseMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}$`)
	//结构体、 []结构体、[][]结构体、map[基础数据类型]结构体、map[基础数据类型][]结构体、map[基础数据类型][][]结构体
	objMapReg       = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]){0,1}\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	objMapArrayReg  = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	objMapArray2Reg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)

	//map[(int8|int16|int32|int64|int)]结构体
	numKVObjMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]){1}\s{0,}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(int8|int16|int32|int64|int)][]结构体
	numKVObjArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(int8|int16|int32|int64|int)][][]结构体
	numKVObj2ArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)

	//map[(float32|float64)]结构体
	floatKVObjMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]){1}\s{0,}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(float32|float64)][]结构体
	floatKVObjArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(float32|float64)][][]结构体
	floatKVObj2ArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)

	//map[(bool)]结构体
	boolKVObjMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]){1}\s{0,}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(bool)][]结构体
	boolKVObjArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[(bool)][][]结构体
	boolKVObj2ArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)

	//map[string]结构体
	strKVObjMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]){1}\s{0,}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[string][]结构体
	strKVObjArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//map[string][][]结构体
	strKVObj2ArrayMapReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]){1}\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//[]结构体
	objArrayMapReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//[][]结构体
	obj2ArrayMapReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)
	//结构体
	objReg = regexp.MustCompile(`^\s{0,}[a-zA-Z0-9_]{1,}\s{0,}$`)

	//lua 基础数据类型
	//number int
	numIntReg = regexp.MustCompile(`^\s{0,}(int8|int16|int32|int64|int)\s{0,}$`)
	//[]number int
	numIntArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	//[][]number int
	num2IntArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)

	//number float
	numFloatReg = regexp.MustCompile(`^\s{0,}(float32|float64)\s{0,}$`)
	//[]number float
	numFloatArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(float32|float64)\s{0,}$`)
	//[][]number float
	num2FloatArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(float32|float64)\s{0,}$`)

	//string
	strReg = regexp.MustCompile(`^\s{0,}(string)\s{0,}$`)
	//[]string
	strArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(string)\s{0,}$`)
	//[][]string
	str2ArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(string)\s{0,}$`)

	//bool
	boolReg = regexp.MustCompile(`^\s{0,}(bool)\s{0,}$`)
	//[]bool
	boolArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(bool)\s{0,}$`)
	//[][]bool
	bool2ArrayReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(bool)\s{0,}$`)

	//lua map[key]value 数据类型
	//map[key]value key=基础数据类型 value=基础数据类型
	baseKNumVNumReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKNumVFloatReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,1}(float32|float64)\s{0,}$`)
	baseKNumVBoolReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,1}(bool)\s{0,}$`)
	baseKNumVStrReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,1}(string)\s{0,}$`)

	baseKFloatVNumReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKFloatVFloatReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,1}(float32|float64)\s{0,}$`)
	baseKFloatVBoolReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,1}(bool)\s{0,}$`)
	baseKFloatVStrReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,1}(string)\s{0,}$`)

	baseKBoolVNumReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKBoolVFloatReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,1}(float32|float64)\s{0,}$`)
	baseKBoolVBoolReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,1}(bool)\s{0,}$`)
	baseKBoolVStrReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,1}(string)\s{0,}$`)

	baseKStrVNumReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKStrVFloatReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,1}(float32|float64)\s{0,}$`)
	baseKStrVBoolReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,1}(bool)\s{0,}$`)
	baseKStrVStrReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,1}(string)\s{0,}$`)

	//map[key]value key=基础数据类型 value=基础数据类型一维数组
	baseKNumVNumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKNumVFloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(float32|float64)\s{0,}$`)
	baseKNumVBoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(bool)\s{0,}$`)
	baseKNumVStrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(string)\s{0,}$`)

	baseKFloatVNumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKFloatVFloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(float32|float64)\s{0,}$`)
	baseKFloatVBoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(bool)\s{0,}$`)
	baseKFloatVStrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(string)\s{0,}$`)

	baseKBoolVNumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKBoolVFloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(float32|float64)\s{0,}$`)
	baseKBoolVBoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(bool)\s{0,}$`)
	baseKBoolVStrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(string)\s{0,}$`)

	baseKStrVNumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKStrVFloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(float32|float64)\s{0,}$`)
	baseKStrVBoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(bool)\s{0,}$`)
	baseKStrVStrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){1}\s{0,1}(string)\s{0,}$`)

	//map[key]value key=基础数据类型 value=基础数据类型二维数组
	baseKNumV2NumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKNumV2FloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(float32|float64)\s{0,}$`)
	baseKNumV2BoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(bool)\s{0,}$`)
	baseKNumV2StrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(string)\s{0,}$`)

	baseKFloatV2NumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKFloatV2FloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(float32|float64)\s{0,}$`)
	baseKFloatV2BoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(bool)\s{0,}$`)
	baseKFloatV2StrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(float32|float64)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(string)\s{0,}$`)

	baseKBoolV2NumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKBoolV2FloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(float32|float64)\s{0,}$`)
	baseKBoolV2BoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(bool)\s{0,}$`)
	baseKBoolV2StrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(string)\s{0,}$`)

	baseKStrV2NumMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(int8|int16|int32|int64|int)\s{0,}$`)
	baseKStrV2FloatMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(float32|float64)\s{0,}$`)
	baseKStrV2BoolMapReg  = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(bool)\s{0,}$`)
	baseKStrV2StrMapReg   = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(string)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){2}\s{0,1}(string)\s{0,}$`)

	//二维数组数据匹配表达式
	arraysValueRegStr    = `^[^\token_begin\token_end]{0,}\token_begin[^\token_begin\token_end]{0,}\token_end[^\token_begin\token_end]{0,}(\array_sep[^\token_begin\token_end]{0,}\token_begin[^\token_begin\token_end]{0,}\token_end[^\token_begin\token_end]{0,}){0,}$`
	arraysSonValueRegStr = `\token_begin[^\token_begin\token_end]{0,}\token_end`
	//map结构的一维数组匹配表达式
	mapArrayValueRegStr = `^[^\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin[^\token_begin\token_end]{0,}\token_end[^\map_sep\token_begin\token_end]{0,}(\array_sep[^\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin[^\token_begin\token_end]{0,}\token_end[^\map_sep\token_begin\token_end]{0,}){0,}$`
	//map结构的一维数组匹配表达式
	mapArraySonValueRegStr = `[^\array_sep\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin[^\token_begin\token_end]{0,}\token_end`

	//map结构的二维数组匹配表达式
	mapArraysValueRegStr    = `^[^\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin([^\map_sep\token_begin\token_end]{0,}\token_begin[^\map_sep\token_begin\token_end]{0,}\token_end[^\map_sep\token_begin\token_end]{0,}){1,}\token_end[^\map_sep\token_begin\token_end]{0,}(\array_sep[^\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin([^\map_sep\token_begin\token_end]{0,}\token_begin[^\map_sep\token_begin\token_end]{0,}\token_end[^\map_sep\token_begin\token_end]{0,}){1,}\token_end[^\map_sep\token_begin\token_end]{0,}){0,}$`
	mapArraysSonValueRegStr = `[^\array_sep\map_sep\token_begin\token_end]{0,}[\map_sep]{1}\s{0,}\token_begin([^\map_sep\token_begin\token_end]{0,}\token_begin[^\map_sep\token_begin\token_end]{0,}\token_end[^\map_sep\token_begin\token_end]{0,}){1,}\token_end`

	arraysValueReg       *regexp.Regexp
	arraysSonValueReg    *regexp.Regexp
	mapArrayValueReg     *regexp.Regexp
	mapArraySonValueReg  *regexp.Regexp
	mapArraysValueReg    *regexp.Regexp
	mapArraysSonValueReg *regexp.Regexp
)

var opts Options = Options{Excels: ExcelsOption{List: make(map[string][]string, 0)}}
var parser = flags.NewParser(&opts, flags.Default)

func init() {
	if args, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			panic(err)
		}
	} else if len(args) > 0 {
		for _, pathfile := range args {
			pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
			sheetName := path.Base(pathfile)
			sheetName = strings.TrimSuffix(sheetName, path.Ext(sheetName))
			for _, v := range opts.Excels.List {
				for _, sn := range v {
					if sn == sheetName {
						panic(fmt.Errorf("duplicate sheet:%+v", sheetName))
					}
				}
			}
			if list, pre := opts.Excels.List[pathfile]; pre {
				list = append(list, sheetName)
			} else {
				list = make([]string, 0)
				list = append(list, sheetName)
				opts.Excels.List[pathfile] = list
			}
		}
	}
	if len(opts.Excels.List) == 0 {
		os.Exit(0)
	}

	MAP_SEPARATOR = opts.MapSeparator
	ARRAY_SEPARATOR = opts.ArraySeparator
	ARRAYS_TOKEN_BEGIN = opts.ArraysTokenBegin
	ARRAYS_TOKEN_END = opts.ArraysTokenEnd
	if opts.Indent != `\t` {
		INDENT = opts.Indent
	}

	arraysValueRegStr = strings.Replace(arraysValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	arraysValueRegStr = strings.Replace(arraysValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	arraysValueRegStr = strings.Replace(arraysValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	//	fmt.Println("arraysValueRegStr=", arraysValueRegStr)
	arraysValueReg = regexp.MustCompile(arraysValueRegStr)

	arraysSonValueRegStr = strings.Replace(arraysSonValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	arraysSonValueRegStr = strings.Replace(arraysSonValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	//	fmt.Println("arraysSonValueRegStr=", arraysSonValueRegStr)
	arraysSonValueReg = regexp.MustCompile(arraysSonValueRegStr)

	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	//	fmt.Println("mapArrayValueRegStr=", mapArrayValueRegStr)
	mapArrayValueReg = regexp.MustCompile(mapArrayValueRegStr)

	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	//	fmt.Println("mapArraySonValueRegStr=", mapArraySonValueRegStr)
	mapArraySonValueReg = regexp.MustCompile(mapArraySonValueRegStr)

	mapArraysValueRegStr = strings.Replace(mapArraysValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArraysValueRegStr = strings.Replace(mapArraysValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArraysValueRegStr = strings.Replace(mapArraysValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArraysValueRegStr = strings.Replace(mapArraysValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	//	fmt.Println("mapArraysValueRegStr=", mapArraysValueRegStr)
	mapArraysValueReg = regexp.MustCompile(mapArraysValueRegStr)

	mapArraysSonValueRegStr = strings.Replace(mapArraysSonValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArraysSonValueRegStr = strings.Replace(mapArraysSonValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArraysSonValueRegStr = strings.Replace(mapArraysSonValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArraysSonValueRegStr = strings.Replace(mapArraysSonValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	//	fmt.Println("mapArraysSonValueRegStr=", mapArraysSonValueRegStr)
	mapArraysSonValueReg = regexp.MustCompile(mapArraysSonValueRegStr)

	if opts.OutGoPath == "" {
		opts.OutGoPath = "./gen_config/sample"
	} else {
		opts.OutGoPath = path.Join(opts.OutGoPath, "sample")
	}
	if opts.OutluaPath == "" {
		opts.OutluaPath = "./lua/sample"
	} else {
		opts.OutluaPath = path.Join(opts.OutluaPath, "sample")
	}
}

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		//构建模板工厂加载器
		generateGoMap(func(s string) {
			file_path := path.Join(opts.OutGoPath, "global_map.go")
			wcgo, err := openFile(file_path)
			if err != nil {
				panic(err)
			}
			defer func() {
				wcgo.Close()
				if e := recover(); e != nil {
					os.Remove(file_path)
					panic(e)
				}
			}()
			if _, err := wcgo.WriteString(s); err != nil {
				panic(err)
			}
		}, func() []string {
			root_sheets := make([]string, 0)
			for _, sheetNames := range opts.Excels.List {
				root_sheets = append(root_sheets, sheetNames...)
			}
			return root_sheets
		})
	}()

	for pathfile, sheetNames := range opts.Excels.List {
		wg.Add(1)
		go func(pathfile string, sheetNames []string) {
			defer wg.Done()
			pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
			className := path.Base(pathfile)
			className = strings.TrimSuffix(className, path.Ext(className))
			xlsxFile, err := xlsx.OpenFile(pathfile)
			if err != nil {
				panic(err)
			}
			file_path := path.Join(opts.OutGoPath, fmt.Sprintf("file_%s.go", className))
			wcgo, err := openFile(file_path)
			if err != nil {
				panic(err)
			}
			defer func() {
				wcgo.Close()
				if e := recover(); e != nil {
					os.Remove(file_path)
					panic(e)
				}
			}()
			printergo := func(s string) {
				if _, err := wcgo.WriteString(s); err != nil {
					panic(err)
				}
			}
			printergo("//Code generated by xlsx-parser.\n")
			printergo("//source: github.com/zxfonline/xlsx_parser\n")
			printergo("//DO NOT EDIT!\n")
			//输出包头
			printergo("\npackage sample\n\n")
			//待解析的标签队列
			parseSheetArray := make([]string, 0, len(sheetNames))
			parseSheetArray = append(parseSheetArray, sheetNames...)
			//加入过解析队列的excel标签
			parsedSheetMap := make(map[string]bool)
			for _, sheetName := range sheetNames {
				parsedSheetMap[sheetName] = true
				if sheet_root, ok := xlsxFile.Sheet[sheetName]; !ok {
					panic(fmt.Errorf("No sheet %s available.\n", sheetName))
				} else { //输出模板工厂
					generateGoFactory(sheet_root, sheetName, printergo)
				}
			}
			//开始输出结构体
			for len(parseSheetArray) > 0 {
				sheetName := parseSheetArray[0]
				parseSheetArray = parseSheetArray[1:]
				addParseSheetArray := generateGoFromXLSXFile(xlsxFile, sheetName, printergo, parsedSheetMap)
				parseSheetArray = append(parseSheetArray, addParseSheetArray...)
				if len(parseSheetArray) == 0 {
					break
				}
			}
		}(pathfile, sheetNames)
	}

	for pathfile, sheetNames := range opts.Excels.List {
		pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
		xlsxFile, err := xlsx.OpenFile(pathfile)
		if err != nil {
			panic(err)
		}
		wg.Add(1)
		go func(xlsxFile *xlsx.File, sheetNames []string) {
			defer wg.Done()
			for _, sheetName := range sheetNames {
				file_path := path.Join(opts.OutluaPath, fmt.Sprintf("sample_%s.lua", sheetName))
				wclua, err := openFile(file_path)
				if err != nil {
					panic(err)
				}
				defer func() {
					wclua.Close()
					if e := recover(); e != nil {
						os.Remove(file_path)
						panic(e)
					}
				}()
				printerlua := func(s string) {
					if _, err := wclua.WriteString(s); err != nil {
						panic(err)
					}
				}
				printerlua("--[[\nCode generated by xlsx-parser.\n")
				printerlua("source: github.com/zxfonline/xlsx_parser\n")
				printerlua("DO NOT EDIT!\n=====attr desc========")
				generateLuaDescFromXLSXFile(xlsxFile, sheetName, printerlua, INDENT)
				printerlua("\n]]\n")
				printerlua(fmt.Sprintf("\nS_%s={", sheetName))
				head := generateLuaHeadFromXLSXFile(xlsxFile, sheetName, printerlua, INDENT)
				generateLuaContentFromXLSXFile(xlsxFile, sheetName, head, printerlua)
				//			fmt.Printf("%+v\n", repr.String(head, repr.Indent("\t")))
				printerlua("\n}\n")
			}
		}(xlsxFile, sheetNames)
	}

	wg.Wait()

	//格式化代码
	if err := exec.Command("gofmt", "-w", opts.OutGoPath).Run(); err != nil {
		panic(fmt.Errorf("go fmt output source file,path:%v ,error:%v", opts.OutGoPath, err))
	}
	//检查代码合法性
	//	if err := exec.Command("go", "build", opts.OutGoPath).Run(); err != nil {
	//		panic(fmt.Errorf("go build output source file,path:%v ,error:%v", opts.OutGoPath, err))
	//	}
	//	if err := exec.Command("goimports", opts.OutGoPath).Run(); err != nil {
	//		panic(err)
	//	}
}

func generateLuaDescFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string), indent string) {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.FormattedValue()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)

		att_type, err := sheet_root.Rows[1].Cells[i].FormattedValue()
		if err != nil {
			panic(err)
		}
		att_type = strings.TrimSpace(att_type)

		att_desc, err := sheet_root.Rows[0].Cells[i].FormattedValue()
		if err != nil {
			panic(err)
		}
		att_desc = strings.TrimSpace(att_desc)

		r, _ := utf8.DecodeRuneInString(att_type)
		if r == '!' {
			continue
		}
		if baseReg.MatchString(att_type) {
			outputf(fmt.Sprintf(`%sP_%s:%s`, fmt.Sprintf("\n%s", indent), att_name, att_desc))
		} else if baseMapReg.MatchString(att_type) {
			outputf(fmt.Sprintf(`%sP_%s:%s`, fmt.Sprintf("\n%s", indent), att_name, att_desc))
		} else if objMapReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			outputf(fmt.Sprintf(`%sP_%s:%s`, fmt.Sprintf("\n%s", indent), att_name, att_desc))
			generateLuaDescFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s", indent, INDENT))
		} else {
			panic(fmt.Errorf(`unknown struct defined "%s"`, att_type))
		}
	}
}

type rowcol struct {
	row       int
	col       int
	att_name  string
	att_type  string
	sheetName string
	indent    string
	son       *rowhead
}
type rowhead struct {
	head map[int]*rowcol
}

func getRowIndex(sheet_root *xlsx.Sheet, sheetName, value string, col int) (int, *xlsx.Row, error) {
	for rowIdx, row := range sheet_root.Rows {
		if rowIdx < 3 {
			continue
		}
		mkvalue, err := row.Cells[col].FormattedValue()
		if err != nil {
			return -1, nil, fmt.Errorf("get row index err:%v,sheet:%v,col:%v,value:%v", err, sheetName, col, value)
		}
		mkvalue = strings.TrimSpace(mkvalue)
		if mkvalue == value {
			return rowIdx, row, nil
		}
	}
	return -1, nil, fmt.Errorf("get row index err:no found field,sheet:%v,col:%v,value:%v", sheetName, col, value)
}
func generateLuaHeadFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string), indent string) *rowhead {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	heads := &rowhead{head: make(map[int]*rowcol)}

	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.FormattedValue()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)

		att_type, err := sheet_root.Rows[1].Cells[i].FormattedValue()
		if err != nil {
			panic(err)
		}
		att_type = strings.TrimSpace(att_type)

		r, _ := utf8.DecodeRuneInString(att_type)
		if r == '!' {
			continue
		}
		rc := &rowcol{
			col:       i,
			att_name:  att_name,
			att_type:  att_type,
			indent:    indent,
			sheetName: sheetName,
		}
		if i != 0 {
			rc.indent = fmt.Sprintf("%s%s", rc.indent, INDENT)
		}
		heads.head[i] = rc
		if baseReg.MatchString(att_type) {
		} else if baseMapReg.MatchString(att_type) {
		} else if objMapArrayReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s%s", rc.indent, INDENT, INDENT))
		} else if objMapArray2Reg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s%s%s", rc.indent, INDENT, INDENT, INDENT))
		} else if obj2ArrayMapReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s%s", rc.indent, INDENT, INDENT))
		} else if objArrayMapReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s", rc.indent, INDENT))
		} else if objReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s", rc.indent))
		} else if objMapReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s", rc.indent, INDENT))
		}
	}
	return heads
}

func generateLuaContentFromXLSXRow(rowIdx int, row *xlsx.Row, heads *rowhead, outputf func(s string), xlsxFile *xlsx.File) {
	for colIdx, cell := range row.Cells {
		if colAttr, pre := heads.head[colIdx]; pre {
			colAttr.row = rowIdx
			att_value, err := cell.FormattedValue()
			if err != nil {
				panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
			}

			if numIntReg.MatchString(colAttr.att_type) { //(int|int8|int16|int32|int64)
				outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.Stoi64(att_value)))
			} else if numIntArrayReg.MatchString(colAttr.att_type) { //[](int|int8|int16|int32|int64)
				if v, err := strutil.ParseInt64s(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
				} else {
					outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.Int64sToStrs(v), ",")))
				}
			} else if num2IntArrayReg.MatchString(colAttr.att_type) { //[][](int|int8|int16|int32|int64)
				if arraysValueReg.MatchString(att_value) {
					att_values := arraysSonValueReg.FindAllString(att_value, -1)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					for i := 0; i < len(att_values); i++ {
						value := strings.TrimSpace(att_values[i])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf("{%v},", strings.Join(strutil.Int64sToStrs(v), ",")))
						}
					}
					outputf("},")
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if numFloatReg.MatchString(colAttr.att_type) { //(float64|float32)
				outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.Stof64(att_value)))
			} else if numFloatArrayReg.MatchString(colAttr.att_type) { //[](float64|float32)
				if v, err := strutil.ParseFloat64s(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
				} else {
					outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.Float64sToStrs(v), ",")))
				}
			} else if num2FloatArrayReg.MatchString(colAttr.att_type) { //[][](float64|float32)
				if arraysValueReg.MatchString(att_value) {
					att_values := arraysSonValueReg.FindAllString(att_value, -1)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					for i := 0; i < len(att_values); i++ {
						value := strings.TrimSpace(att_values[i])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf("{%v},", strings.Join(strutil.Float64sToStrs(v), ",")))
						}
					}
					outputf("},")
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if strReg.MatchString(colAttr.att_type) { //string
				outputf(fmt.Sprintf("%sP_%s=[[%v]],", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, att_value))
			} else if strArrayReg.MatchString(colAttr.att_type) { //[]string
				outputf(fmt.Sprintf("%sP_%s={[[%v]]},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strings.Split(att_value, ARRAY_SEPARATOR), "]],[[")))
			} else if str2ArrayReg.MatchString(colAttr.att_type) { //[][]string
				if arraysValueReg.MatchString(att_value) {
					att_values := arraysSonValueReg.FindAllString(att_value, -1)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					for i := 0; i < len(att_values); i++ {
						value := strings.TrimSpace(att_values[i])
						value = value[1 : len(value)-1]
						outputf(fmt.Sprintf("{[[%v]]},", strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
					}
					outputf("},")
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if boolReg.MatchString(colAttr.att_type) { //bool
				outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.StoBol(att_value)))
			} else if boolArrayReg.MatchString(colAttr.att_type) { //[]bool
				if v, err := strutil.ParseBools(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
				} else {
					outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.BoolsToStrs(v), ",")))
				}
			} else if bool2ArrayReg.MatchString(colAttr.att_type) { //[][]bool
				if arraysValueReg.MatchString(att_value) {
					att_values := arraysSonValueReg.FindAllString(att_value, -1)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					for i := 0; i < len(att_values); i++ {
						value := strings.TrimSpace(att_values[i])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf("{%v},", strings.Join(strutil.BoolsToStrs(v), ",")))
						}
					}
					outputf("},")
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumVNumReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)](int|int8|int16|int32|int64)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[int64]int64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k, v int64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stoi64(kv[0])
					v = strutil.Stoi64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKNumVNumMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][](int|int8|int16|int32|int64)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumV2NumMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][][](int|int8|int16|int32|int64)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Int64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumVFloatReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)](float64|float32)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[int64]float64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k int64
				var v float64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stoi64(kv[0])
					v = strutil.Stof64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKNumVFloatMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][](float64|float32)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumV2FloatMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][][](float64|float32)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Float64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumVBoolReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)]bool
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[int64]bool)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k int64
				var v bool
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stoi64(kv[0])
					v = strutil.StoBol(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKNumVBoolMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][]bool
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumV2BoolMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][][]bool
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.BoolsToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumVStrReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)]string
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[int64]string)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k int64
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stoi64(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKNumVStrMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][]string
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKNumV2StrMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][][]string
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							outputf(fmt.Sprintf("%s{[[%v]]},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatVNumReg.MatchString(colAttr.att_type) { //map[(float64|float32)](int|int8|int16|int32|int64)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[float64]int64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k float64
				var v int64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stof64(kv[0])
					v = strutil.Stoi64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKFloatVNumMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][](int|int8|int16|int32|int64)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatV2NumMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][][](int|int8|int16|int32|int64)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Int64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatVFloatReg.MatchString(colAttr.att_type) { //map[(float64|float32)](float64|float32)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[float64]float64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k, v float64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stof64(kv[0])
					v = strutil.Stof64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKFloatVFloatMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][](float64|float32)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatV2FloatMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][][](float64|float32)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Float64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatVBoolReg.MatchString(colAttr.att_type) { //map[(float64|float32)]string
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[float64]bool)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k float64
				var v bool
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stof64(kv[0])
					v = strutil.StoBol(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKFloatVBoolMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][]bool
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatV2BoolMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][][]bool
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.BoolsToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatVStrReg.MatchString(colAttr.att_type) { //map[(float64|float32)]string
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[float64]string)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k float64
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stof64(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKFloatVStrMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][]string
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKFloatV2StrMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][][]string
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							outputf(fmt.Sprintf("%s{[[%v]]},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolVNumReg.MatchString(colAttr.att_type) { //map[bool](int|int8|int16|int32|int64)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[bool]int64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k bool
				var v int64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.StoBol(kv[0])
					v = strutil.Stoi64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKBoolVNumMapReg.MatchString(colAttr.att_type) { //map[bool][](int|int8|int16|int32|int64)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if _, pre := result[k]; pre {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolV2NumMapReg.MatchString(colAttr.att_type) { //map[bool][][](int|int8|int16|int32|int64)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Int64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolVFloatReg.MatchString(colAttr.att_type) { //map[bool](float64|float32)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[bool]float64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k bool
				var v float64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.StoBol(kv[0])
					v = strutil.Stof64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKBoolVFloatMapReg.MatchString(colAttr.att_type) { //map[bool][](float64|float32)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if _, pre := result[k]; pre {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolV2FloatMapReg.MatchString(colAttr.att_type) { //map[bool][][](float64|float32)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Float64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolVBoolReg.MatchString(colAttr.att_type) { //map[bool]bool
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[bool]bool)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k, v bool
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.StoBol(kv[0])
					v = strutil.StoBol(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKBoolVBoolMapReg.MatchString(colAttr.att_type) { //map[bool][]bool
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if _, pre := result[k]; pre {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolV2BoolMapReg.MatchString(colAttr.att_type) { //map[bool][][]bool
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.BoolsToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolVStrReg.MatchString(colAttr.att_type) { //map[bool]string
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[bool]string)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k bool
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.StoBol(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v
					outputf(fmt.Sprintf(`%s["%v"]=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKBoolVStrMapReg.MatchString(colAttr.att_type) { //map[bool][]string
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if _, pre := result[k]; pre {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKBoolV2StrMapReg.MatchString(colAttr.att_type) { //map[bool][][]string
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							outputf(fmt.Sprintf("%s{[[%v]]},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrVNumReg.MatchString(colAttr.att_type) { //map[string](int|int8|int16|int32|int64)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[string]int64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k string
				var v int64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strings.TrimSpace(kv[0])
					v = strutil.Stoi64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v

					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKStrVNumMapReg.MatchString(colAttr.att_type) { //map[string][](int|int8|int16|int32|int64)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrV2NumMapReg.MatchString(colAttr.att_type) { //map[string][][](int|int8|int16|int32|int64)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseInt64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Int64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrVFloatReg.MatchString(colAttr.att_type) { //map[string](float32|float64)
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[string]float64)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k string
				var v float64
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strings.TrimSpace(kv[0])
					v = strutil.Stof64(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v

					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKStrVFloatMapReg.MatchString(colAttr.att_type) { //map[string][](float32|float64)
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrV2FloatMapReg.MatchString(colAttr.att_type) { //map[string][][](float32|float64)
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseFloat64s(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.Float64sToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrVBoolReg.MatchString(colAttr.att_type) { //map[string]bool
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[string]bool)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k string
				var v bool
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strings.TrimSpace(kv[0])
					v = strutil.StoBol(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v

					outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKStrVBoolMapReg.MatchString(colAttr.att_type) { //map[string][]bool
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrV2BoolMapReg.MatchString(colAttr.att_type) { //map[string][][]bool
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							if v, err := strutil.ParseBools(strings.Split(value, ARRAY_SEPARATOR)); err != nil {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
							} else {
								outputf(fmt.Sprintf("%s{%v},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strutil.BoolsToStrs(v), ",")))
							}
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrVStrReg.MatchString(colAttr.att_type) { //map[string]string
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[string]string)
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				var k, v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strings.TrimSpace(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = v

					outputf(fmt.Sprintf(`%s["%v"]=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if baseKStrVStrMapReg.MatchString(colAttr.att_type) { //map[string][]string
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]
						outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if baseKStrV2StrMapReg.MatchString(colAttr.att_type) { //map[string][][]string
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]
							outputf(fmt.Sprintf("%s{[[%v]]},", fmt.Sprintf("\n%s%s%s", colAttr.indent, INDENT, INDENT), strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if numKVObjMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)]子对象
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[int64]bool)
				var k int64
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stoi64(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = true
					if colAttr.son == nil {
						panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
					}
					son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
					son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
					if ok == false {
						panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
					}
					srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
					if err != nil {
						panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
					}

					outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
					generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
					outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if numKVObjArrayMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][]子对象
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]

						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						vs := strings.Split(value, ARRAY_SEPARATOR)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, v := range vs {
							srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
							if err != nil {
								panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
							}
							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if numKVObj2ArrayMapReg.MatchString(colAttr.att_type) { //map[(int|int8|int16|int32|int64)][][]子对象
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[int64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k int64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stoi64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]

							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							vs := strings.Split(value, ARRAY_SEPARATOR)
							for _, v := range vs {
								srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
								if err != nil {
									panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
								}
								outputf(fmt.Sprintf("\n%s%s%s%s{", colAttr.indent, INDENT, INDENT, INDENT))
								generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
								outputf(fmt.Sprintf("\n%s%s%s%s},", colAttr.indent, INDENT, INDENT, INDENT))
							}
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if floatKVObjMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)]子对象
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[float64]bool)
				var k float64
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.Stof64(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = true
					if colAttr.son == nil {
						panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
					}
					son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
					son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
					if ok == false {
						panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
					}
					srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
					if err != nil {
						panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
					}

					outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
					generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
					outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if floatKVObjArrayMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][]子对象
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]

						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						vs := strings.Split(value, ARRAY_SEPARATOR)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, v := range vs {
							srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
							if err != nil {
								panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
							}
							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if floatKVObj2ArrayMapReg.MatchString(colAttr.att_type) { //map[(float64|float32)][][]子对象
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[float64]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k float64
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.Stof64(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]

							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							vs := strings.Split(value, ARRAY_SEPARATOR)
							for _, v := range vs {
								srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
								if err != nil {
									panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
								}
								outputf(fmt.Sprintf("\n%s%s%s%s{", colAttr.indent, INDENT, INDENT, INDENT))
								generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
								outputf(fmt.Sprintf("\n%s%s%s%s},", colAttr.indent, INDENT, INDENT, INDENT))
							}
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if boolKVObjMapReg.MatchString(colAttr.att_type) { //map[bool]子对象
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[bool]bool)
				var k bool
				var v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strutil.StoBol(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = true
					if colAttr.son == nil {
						panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
					}
					son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
					son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
					if ok == false {
						panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
					}
					srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
					if err != nil {
						panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
					}

					outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
					generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
					outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if boolKVObjArrayMapReg.MatchString(colAttr.att_type) { //map[bool][]子对象
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]

						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						vs := strings.Split(value, ARRAY_SEPARATOR)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, v := range vs {
							srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
							if err != nil {
								panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
							}
							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if boolKVObj2ArrayMapReg.MatchString(colAttr.att_type) { //map[bool][][]子对象
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[bool]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k bool
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strutil.StoBol(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]

							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							vs := strings.Split(value, ARRAY_SEPARATOR)
							for _, v := range vs {
								srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
								if err != nil {
									panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
								}
								outputf(fmt.Sprintf("\n%s%s%s%s{", colAttr.indent, INDENT, INDENT, INDENT))
								generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
								outputf(fmt.Sprintf("\n%s%s%s%s},", colAttr.indent, INDENT, INDENT, INDENT))
							}
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if strKVObjMapReg.MatchString(colAttr.att_type) { //map[string]子对象
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				kvsm := strings.Split(att_value, ARRAY_SEPARATOR)
				result := make(map[string]bool)
				var k, v string
				for _, kvs := range kvsm {
					if strings.TrimSpace(kvs) == "" {
						continue
					}
					kv := strings.Split(kvs, MAP_SEPARATOR)
					if len(kv) != 2 {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
					k = strings.TrimSpace(kv[0])
					v = strings.TrimSpace(kv[1])
					if _, pre := result[k]; pre {
						panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
					}
					result[k] = true
					if colAttr.son == nil {
						panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
					}
					son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
					son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
					if ok == false {
						panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
					}
					srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
					if err != nil {
						panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
					}

					outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
					generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
					outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if strKVObjArrayMapReg.MatchString(colAttr.att_type) { //map[string][]子对象
				if mapArrayValueReg.MatchString(att_value) {
					kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvs := range kvsm {
						kv := strings.Split(kvs, MAP_SEPARATOR)
						if len(kv) != 2 {
							//							panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						value := strings.TrimSpace(kv[1])
						value = value[1 : len(value)-1]

						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						vs := strings.Split(value, ARRAY_SEPARATOR)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, v := range vs {
							srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
							if err != nil {
								panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
							}
							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if strKVObj2ArrayMapReg.MatchString(colAttr.att_type) { //map[string][][]子对象
				if mapArraysValueReg.MatchString(att_value) {
					kvsms := mapArraysSonValueReg.FindAllString(att_value, -1)
					result := make(map[string]bool)
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					var k string
					for _, kvas := range kvsms {
						kv := strings.Split(kvas, MAP_SEPARATOR)
						if len(kv) != 2 {
							//panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							continue
						}
						k = strings.TrimSpace(kv[0])
						if ex := result[k]; ex {
							panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
						}
						result[k] = true
						if colAttr.son == nil {
							panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
						}
						son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
						son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
						if ok == false {
							panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
						}
						arrays := arraysSonValueReg.FindAllString(kv[1], -1)
						outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k))
						for _, value := range arrays {
							value = strings.TrimSpace(value)
							value = value[1 : len(value)-1]

							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							vs := strings.Split(value, ARRAY_SEPARATOR)
							for _, v := range vs {
								srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
								if err != nil {
									panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
								}
								outputf(fmt.Sprintf("\n%s%s%s%s{", colAttr.indent, INDENT, INDENT, INDENT))
								generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
								outputf(fmt.Sprintf("\n%s%s%s%s},", colAttr.indent, INDENT, INDENT, INDENT))
							}
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if objArrayMapReg.MatchString(colAttr.att_type) { //[]子对象
				att_value = strings.TrimSpace(att_value)
				if colAttr.son == nil {
					panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
				}
				son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
				son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
				if ok == false {
					panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
				}
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				vs := strings.Split(att_value, ARRAY_SEPARATOR)
				for _, v := range vs {
					v = strings.TrimSpace(v)
					if v == "" {
						continue
					}
					srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
					if err != nil {
						panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
					}
					outputf(fmt.Sprintf("\n%s%s{", colAttr.indent, INDENT))
					generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
					outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
				}
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			} else if obj2ArrayMapReg.MatchString(colAttr.att_type) { //[][]子对象
				if arraysValueReg.MatchString(att_value) {
					att_values := arraysSonValueReg.FindAllString(att_value, -1)
					son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
					son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
					if ok == false {
						panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
					}
					outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
					for i := 0; i < len(att_values); i++ {
						value := strings.TrimSpace(att_values[i])
						value = value[1 : len(value)-1]
						vs := strings.Split(value, ARRAY_SEPARATOR)
						outputf(fmt.Sprintf("\n%s%s{", colAttr.indent, INDENT))
						for _, v := range vs {
							v = strings.TrimSpace(v)
							if v == "" {
								continue
							}
							srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, v, MAINKEY_INDEX)
							if err != nil {
								panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
							}
							outputf(fmt.Sprintf("\n%s%s%s{", colAttr.indent, INDENT, INDENT))
							generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
							outputf(fmt.Sprintf("\n%s%s%s},", colAttr.indent, INDENT, INDENT))
						}
						outputf(fmt.Sprintf("\n%s%s},", colAttr.indent, INDENT))
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
				}
			} else if objReg.MatchString(colAttr.att_type) { //子对象
				outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
				att_value = strings.TrimSpace(att_value)
				if colAttr.son == nil {
					panic(fmt.Errorf("invalid field,no found son sheet,loc:%+v", colAttr))
				}
				son_sheetName := colAttr.son.head[MAINKEY_INDEX].sheetName
				son_sheet_root, ok := xlsxFile.Sheet[son_sheetName]
				if ok == false {
					panic(fmt.Errorf("No sheet %s available.\n", son_sheetName))
				}
				srowIdx, srow, err := getRowIndex(son_sheet_root, son_sheetName, att_value, MAINKEY_INDEX)
				if err != nil {
					panic(fmt.Errorf("invalid field,loc:%+v,err:%v", colAttr, err))
				}
				generateLuaContentFromXLSXRow(srowIdx, srow, colAttr.son, outputf, xlsxFile)
				outputf(fmt.Sprintf("\n%s},", colAttr.indent))
			}
		}
	}
}

func generateLuaContentFromXLSXFile(xlsxFile *xlsx.File, sheetName string, heads *rowhead, outputf func(s string)) {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	hash := make(map[string]bool)
	for rowIdx, row := range sheet_root.Rows {
		if rowIdx < 3 {
			continue
		}
		//主键处理
		mk := heads.head[MAINKEY_INDEX]
		mk.row = rowIdx
		mkvalue, err := row.Cells[MAINKEY_INDEX].FormattedValue()
		if err != nil {
			panic(fmt.Errorf("invalid main key value,loc:%+v ,err:%v", mk, err))
		}
		mkvalue = strings.TrimSpace(mkvalue)
		if hash != nil {
			if hash[mkvalue] {
				panic(fmt.Errorf("duplicate main key's value in field: %s,loc:%+v", mkvalue, mk))
			}
			hash[mkvalue] = true
		}
		outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s", mk.indent), mkvalue))
		generateLuaContentFromXLSXRow(rowIdx, row, heads, outputf, xlsxFile)
		outputf(fmt.Sprintf("\n%s},", mk.indent))
	}
}

func generateGoFactory(sheet_root *xlsx.Sheet, sheetName string, outputf func(s string)) {
	keyname, err := sheet_root.Rows[2].Cells[MAINKEY_INDEX].FormattedValue()
	if err != nil {
		panic(err)
	}
	keyname = strings.TrimSpace(keyname)
	keytype, err := sheet_root.Rows[1].Cells[MAINKEY_INDEX].FormattedValue()
	if err != nil {
		panic(err)
	}
	keytype = strings.TrimSpace(keytype)
	tmpl := template.Must(template.New("codeBaseTemplate").Parse(`
	type SF_{{.Name}} map[{{.KeyType}}]*S_{{.Name}}

	//获取模板数据(请勿在模板数据上修改数据) sid 数据类型={{.KeyType}}
	func (f SF_{{.Name}}) Get(sid interface{}) Sample {
		if s, pre := f[sid.({{.KeyType}})]; pre {
			return s
		}
		return nil
	}
	// sid 数据类型={{.KeyType}}
	func (s *S_{{.Name}}) Sid() interface{} {
		return s.P_{{.KeyName}}
	}
	`))
	var bs bytes.Buffer
	if err := tmpl.Execute(&bs, struct {
		Name    string
		KeyName string
		KeyType string
	}{sheetName, keyname, keytype}); err != nil {
		panic(err)
	}
	outputf(fmt.Sprintf("%s\n", bs.String()))
}

func generateGoFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string), parsedSheetMap map[string]bool) (addParseSheetArray []string) {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	outputf(fmt.Sprintf("type S_%s struct {\n", sheetName))
	hash := make(map[string]bool)
	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.FormattedValue()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)
		if hash[att_name] {
			panic(fmt.Errorf(" sheet[%s] duplicate field name in struct literal: %s", sheetName, att_name))
		}
		hash[att_name] = true

		att_type, err := sheet_root.Rows[1].Cells[i].FormattedValue()
		if err != nil {
			panic(err)
		}
		att_type = strings.TrimSpace(att_type)

		att_desc, err := sheet_root.Rows[0].Cells[i].FormattedValue()
		if err != nil {
			panic(err)
		}
		att_desc = strings.TrimSpace(att_desc)

		r, _ := utf8.DecodeRuneInString(att_type)
		if r == '!' {
			continue
		}

		outputf("\t/*")
		outputf(att_desc)
		outputf("*/\n")

		if baseReg.MatchString(att_type) {
			outputf(fmt.Sprintf("\tP_%s %s\n", att_name, att_type))
		} else if baseMapReg.MatchString(att_type) {
			outputf(fmt.Sprintf("\tP_%s %s\n", att_name, att_type))
		} else if objMapReg.MatchString(att_type) {
			son_sheetName := att_type
			base := ""
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
				base = att_type[:idx+1]
			}
			if _, ok := parsedSheetMap[son_sheetName]; !ok {
				parsedSheetMap[son_sheetName] = true
				addParseSheetArray = append(addParseSheetArray, son_sheetName)
			}
			outputf(fmt.Sprintf("\tP_%s %sS_%s\n", att_name, base, son_sheetName))
		} else {
			panic(fmt.Errorf(`unknown struct defined "%s"`, att_type))
		}
	}
	outputf("}\n")
	return
}

func generateGoMap(outputf func(s string), Factory func() []string) {
	tmpl := template.Must(template.New("codeGoMapTemplate").Parse(`
//Code generated by xlsx-parser.
//source: github.com/zxfonline/xlsx_parser
//DO NOT EDIT!

package sample

import (
	"reflect"

	"sync"

	"github.com/zxfonline/golog"
)

var (
	_FACTORY_BUILDERS map[SampleKey]*sampleFactoryBuilder
	_GLOBAL_MAP       map[SampleKey]SampleFactory

	_GLOBAL_NAMEKEY map[string]SampleKey

	log = golog.New("SampleFactory")

	mapLock     sync.RWMutex
	builderLock sync.RWMutex
)

type SampleKey int

const (
	SampleKey_Begin = SampleKey(0) + iota
	{{range .}}SampleKey_SF_{{.}}{{"\n"}}{{end}}
)

func init() {
	_FACTORY_BUILDERS = make(map[SampleKey]*sampleFactoryBuilder)
	_GLOBAL_MAP = make(map[SampleKey]SampleFactory)
	_GLOBAL_NAMEKEY = make(map[string]SampleKey)
	//初始化模板名对应的模板key
	{{range .}}_GLOBAL_NAMEKEY["SF_{{.}}"] = SampleKey_SF_{{.}}{{"\n"}}{{end}}
	//配置模板注册
	{{range .}}RegistSampleFactoryBuilder(&SF_{{.}}{}){{"\n"}}{{end}}
}

type sampleFactoryBuilder struct {
	typeOf reflect.Type
}

// only support pointer of a struct or a struct
// &ta{} -> ta,ta{} -> ta
// for debug use fmt.Printf("%T",xxx)
func RegistSampleFactoryBuilder(factory SampleFactory) {
	builderLock.Lock()
	defer builderLock.Unlock()
	typof := IndirectType(reflect.TypeOf(factory))
	name := typof.Name()
	mk := _GLOBAL_NAMEKEY[name]
	//注册handler
	if _, pre := _FACTORY_BUILDERS[mk]; pre {
		log.Infof("Replace sample builder \"%s\"", name)
	} else {
		log.Infof("Add sample builder \"%s\"", name)
	}
	_FACTORY_BUILDERS[mk] = &sampleFactoryBuilder{typeOf: typof}
}

func IndirectType(v reflect.Type) reflect.Type {
	switch v.Kind() {
	case reflect.Ptr:
		return IndirectType(v.Elem())
	default:
		return v
	}
	return v
}

func InstanceSFBuilder(name string) SampleFactory {
	builderLock.RLock()
	defer builderLock.RUnlock()
	if sample, pre := _FACTORY_BUILDERS[_GLOBAL_NAMEKEY[name]]; pre {
		return reflect.New(sample.typeOf).Interface().(SampleFactory)
	} else {
		return nil
	}
}

func GetSampleFactory(name SampleKey) SampleFactory {
	mapLock.RLock()
	defer mapLock.RUnlock()
	if sf, pre := _GLOBAL_MAP[name]; pre {
		return sf
	} else {
		return nil
	}
}

func DynamicUpdateSampleFactory(name string, sf SampleFactory) {
	mapLock.Lock()
	defer mapLock.Unlock()
	mk := _GLOBAL_NAMEKEY[name]
	if _, pre := _GLOBAL_MAP[mk]; pre {
		log.Infof("Replace sample factory \"%s\"", name)
	} else {
		log.Infof("Add sample factory \"%s\"", name)
	}
	_GLOBAL_MAP[mk] = sf
}

//模板接口
type Sample interface {
	Sid() interface{}
}

type SampleFactory interface {
	Get(sid interface{}) Sample
}
	`))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, Factory())
	if err != nil {
		panic(err)
	}
	outputf(buf.String())
}

func openFile(pathfile string) (wc *os.File, err error) {
	dir, _ := path.Split(pathfile)
	if _, err = os.Stat(dir); err != nil && !os.IsExist(err) {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err = os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, err
		}
		if _, err = os.Stat(dir); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(pathfile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, os.ModePerm)
}
