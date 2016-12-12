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

	//	"github.com/alecthomas/repr"
	"text/template"

	"github.com/tealeg/xlsx"
	"github.com/zxfonline/strutil"

	"bytes"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	OutGoPath        string `short:"g" long:"dgo" description:"golang 源文件输出目录"`
	OutluaPath       string `short:"l" long:"dlua" description:"lua 源文件输出目录"`
	MapSeparator     string `short:"m" long:"map_sep" default:"=" description:"map key=value 分隔符 默认 = "`
	ArraySeparator   string `short:"a" long:"array_sep" default:"," description:"数组内容 分隔符 默认 , "`
	ArraysTokenBegin string `short:"b" long:"token_begin" default:"[" description:"二维数组节点开始标记 默认 [ "`
	ArraysTokenEnd   string `short:"e" long:"token_end" default:"]" description:"二维数组节点开始标记 默认 ] "`
	Indent           string `short:"i" long:"indent" default:"\t" description:"节点间缩进  默认 \t "`
}

var (
	sheetMap   map[string]bool
	parseArray []string
	//map key=value 默认分隔符
	MAP_SEPARATOR = "="

	//数组 [1,2,3] 默认分隔符
	ARRAY_SEPARATOR = ","

	//二维数组节点开始标记
	ARRAYS_TOKEN_BEGIN = "["
	//二维数组节点结束标记
	ARRAYS_TOKEN_END = "]"

	INDENT = "\t"
)

var (
	//golang 数据类型处理

	//基础数据类型(int8、int16、int32、int64、int、float32、float64、string、bool) 和 对应的数值(一维、二维)eg:int、[]int、[][]int
	baseReg = regexp.MustCompile(`^\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}$`)
	//map[key]value key=基础数据类型 value=基础数据类型 、 基础数据类型 一维数组 、 基础数据类型 二维数组 eg:map[int]int、map[int][]int、map[int][][]int
	baseMapReg = regexp.MustCompile(`^\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}$`)
	//结构体、 []结构体、[][]结构体、map[基础数据类型]结构体、map[基础数据类型][]结构体、map[基础数据类型][][]结构体
	objMapArrayReg = regexp.MustCompile(`^(\s{0,}map\s{0,}\[\s{0,}(int8|int16|int32|int64|int|float32|float64|string|bool)\s{0,}\]){0,1}\s{0,}(\[\s{0,}\]\s{0,}){0,2}\s{0,1}[a-zA-Z0-9_]{1,}\s{0,}$`)

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

	//主键数据内容匹配类型
	mainKeyReg = regexp.MustCompile(`^\s{0,}[a-zA-Z_][a-zA-Z0-9_]{0,}\s{0,}$`)

	//二维数组数据匹配表达式
	arraysValueRegStr    = `^\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}(\array_sep\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}){0,}(\array_sep){0,}$`
	arraysSonValueRegStr = `\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}`
	//map结构的一维数组匹配表达式
	mapArrayValueRegStr = `^\s{0,}[^\token_begin\token_end]{0,}\s{0,}[\map_sep]{1}\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}(\array_sep\s{0,}[^\token_begin\token_end]{0,}\s{0,}[\map_sep]{1}\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}){0,}(\array_sep){0,}$`
	//map结构的一维数组匹配表达式
	mapArraySonValueRegStr = `\s{0,}[^\array_sep\token_begin\token_end]{0,}\s{0,}[\map_sep]{1}\s{0,}\token_begin\s{0,}[^\token_begin\token_end]{0,}\s{0,}\token_end\s{0,}`

	arraysValueReg      *regexp.Regexp
	arraysSonValueReg   *regexp.Regexp
	mapArrayValueReg    *regexp.Regexp
	mapArraySonValueReg *regexp.Regexp
)

func main() {
	args, err := flags.ParseArgs(&opts, os.Args[1:])
	if err != nil {
		panic(err)
	}

	MAP_SEPARATOR = opts.MapSeparator
	ARRAY_SEPARATOR = opts.ArraySeparator
	ARRAYS_TOKEN_BEGIN = opts.ArraysTokenBegin
	ARRAYS_TOKEN_END = opts.ArraysTokenEnd
	INDENT = opts.Indent

	arraysValueRegStr = strings.Replace(arraysValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	arraysValueRegStr = strings.Replace(arraysValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	arraysValueRegStr = strings.Replace(arraysValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	arraysValueReg = regexp.MustCompile(arraysValueRegStr)

	arraysSonValueRegStr = strings.Replace(arraysSonValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	arraysSonValueRegStr = strings.Replace(arraysSonValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	arraysSonValueReg = regexp.MustCompile(arraysSonValueRegStr)

	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArrayValueRegStr = strings.Replace(mapArrayValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	mapArrayValueReg = regexp.MustCompile(mapArrayValueRegStr)

	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "token_begin", ARRAYS_TOKEN_BEGIN, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "token_end", ARRAYS_TOKEN_END, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "array_sep", ARRAY_SEPARATOR, -1)
	mapArraySonValueRegStr = strings.Replace(mapArraySonValueRegStr, "map_sep", MAP_SEPARATOR, -1)
	mapArraySonValueReg = regexp.MustCompile(mapArraySonValueRegStr)

	gopath := opts.OutGoPath
	if gopath == "" {
		gopath = "./gen_config/sample"
	} else {
		gopath = path.Join(gopath, "gen_config/sample")
	}
	luapath := opts.OutluaPath
	if luapath == "" {
		luapath = "./luaScript/sample"
	} else {
		luapath = path.Join(luapath, "luaScript/sample")
	}
	files_excels := args
	for _, pathfile := range files_excels {
		sheetMap = make(map[string]bool)
		pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
		sheetName := path.Base(pathfile)
		sheetName = strings.TrimSuffix(sheetName, path.Ext(sheetName))
		xlsxFile, err := xlsx.OpenFile(pathfile)
		if err != nil {
			panic(err)
		}
		parseArray = append(parseArray, sheetName)
		sheetMap[sheetName] = true

		func() {
			file_path := path.Join(gopath, fmt.Sprintf("sample_%s.go", strings.ToLower(sheetName)))
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
			printergo("//Code generated by protoc-gen-go.\n")
			printergo("//source: github.com/zxfonline/xlsx_parser\n")
			printergo("//DO NOT EDIT!\n")
			//输出包头
			printergo("\npackage sample\n\n")

			for len(parseArray) > 0 {
				sheetName = parseArray[0]
				parseArray = parseArray[1:]
				generateGoFromXLSXFile(xlsxFile, sheetName, printergo)
				if len(parseArray) == 0 {
					break
				}
			}
		}()
	}
	generateGoMap(func(s string) {
		file_path := path.Join(gopath, "global_map.go")
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
	})

	//格式化代码
	if err := exec.Command("gofmt", "-w", gopath).Run(); err != nil {
		panic(fmt.Errorf("go fmt output source file,path:%v ,error:%v", gopath, err))
	}
	//检查代码合法性
	//	if err := exec.Command("go", "build", gopath).Run(); err != nil {
	//		panic(fmt.Errorf("go build output source file,path:%v ,error:%v", gopath, err))
	//	}
	//	if err := exec.Command("goimports", gopath).Run(); err != nil {
	//		panic(err)
	//	}

	for _, pathfile := range files_excels {
		pathfile = strings.Replace(filepath.Clean(pathfile), "\\", "/", -1)
		sheetName := path.Base(pathfile)
		sheetName = strings.TrimSuffix(sheetName, path.Ext(sheetName))
		xlsxFile, err := xlsx.OpenFile(pathfile)
		if err != nil {
			panic(err)
		}
		func() {
			file_path := path.Join(luapath, fmt.Sprintf("sample_%s.lua", sheetName))
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
			printerlua("--[[\nCode generated by protoc-gen-go.\n")
			printerlua("source: github.com/zxfonline/xlsx_parser\n")
			printerlua("DO NOT EDIT!\n=====attr desc========")
			generateLuaDescFromXLSXFile(xlsxFile, sheetName, printerlua, INDENT)
			printerlua("\n]]\n")
			printerlua(fmt.Sprintf("\nS_%s={", sheetName))
			head := generateLuaHeadFromXLSXFile(xlsxFile, sheetName, printerlua, INDENT)
			generateLuaContentFromXLSXFile(xlsxFile, sheetName, head, printerlua)
			//						fmt.Printf("%+v\n", repr.Repr(head, repr.Indent(INDENT)))
			printerlua("\n}\n")
		}()
	}
}

func generateLuaDescFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string), indent string) {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.String()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)

		att_type, err := sheet_root.Rows[1].Cells[i].String()
		if err != nil {
			panic(err)
		}
		att_type = strings.TrimSpace(att_type)

		att_desc, err := sheet_root.Rows[0].Cells[i].String()
		if err != nil {
			panic(err)
		}
		att_desc = strings.TrimSpace(att_desc)

		r, _ := utf8.DecodeRuneInString(att_type)
		if r == '!' {
			continue
		}
		if baseReg.MatchString(att_type) {
			outputf(fmt.Sprintf("%sP_%s:%s", fmt.Sprintf("\n%s", indent), att_name, att_desc))
		} else if baseMapReg.MatchString(att_type) {
			outputf(fmt.Sprintf("%sP_%s:%s", fmt.Sprintf("\n%s", indent), att_name, att_desc))
		} else if objMapArrayReg.MatchString(att_type) {
			son_sheetName := att_type
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
			}
			outputf(fmt.Sprintf("%sP_%s:%s", fmt.Sprintf("\n%s", indent), att_name, att_desc))
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

func newrh() *rowhead {
	return &rowhead{head: make(map[int]*rowcol)}
}

func generateLuaHeadFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string), indent string) *rowhead {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	heads := newrh()

	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.String()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)

		att_type, err := sheet_root.Rows[1].Cells[i].String()
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
			rc.son = generateLuaHeadFromXLSXFile(xlsxFile, son_sheetName, outputf, fmt.Sprintf("%s%s", rc.indent, INDENT))
		}
	}
	return heads
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
		mk := heads.head[0]
		mk.row = rowIdx
		mkvalue, err := row.Cells[0].String()
		if err != nil {
			panic(fmt.Errorf("invalid main key value,loc:%+v ,err:%v", mk, err))
		}
		mkvalue = strings.TrimSpace(mkvalue)

		if hash[mkvalue] {
			panic(fmt.Errorf("duplicate main key's value in field: %s,loc:%+v", mkvalue, mk))
		}
		hash[mkvalue] = true
		if mainKeyReg.MatchString(mkvalue) { //字符串类型
			outputf(fmt.Sprintf(`%s%v={`, fmt.Sprintf("\n%s", mk.indent), mkvalue))
		} else {
			outputf(fmt.Sprintf(`%s["%v"]={`, fmt.Sprintf("\n%s", mk.indent), mkvalue))
		}

		for colIdx, cell := range row.Cells {
			if colAttr, pre := heads.head[colIdx]; pre {
				colAttr.row = rowIdx
				att_value, err := cell.String()
				if err != nil {
					panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
				}

				if numIntReg.MatchString(colAttr.att_type) {
					outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.Stoi64(att_value)))
				} else if numIntArrayReg.MatchString(colAttr.att_type) {
					if v, err := strutil.ParseInt64s(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
					} else {
						outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.Int64sToStrs(v), ",")))
					}
				} else if num2IntArrayReg.MatchString(colAttr.att_type) {
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
				} else if numFloatReg.MatchString(colAttr.att_type) {
					outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.Stof64(att_value)))
				} else if numFloatArrayReg.MatchString(colAttr.att_type) {
					if v, err := strutil.ParseFloat64s(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
					} else {
						outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.Float64sToStrs(v), ",")))
					}
				} else if num2FloatArrayReg.MatchString(colAttr.att_type) {
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
				} else if strReg.MatchString(colAttr.att_type) {
					outputf(fmt.Sprintf("%sP_%s=[[%v]],", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, att_value))
				} else if strArrayReg.MatchString(colAttr.att_type) {
					outputf(fmt.Sprintf("%sP_%s={[[%v]]},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strings.Split(att_value, ARRAY_SEPARATOR), "]],[[")))
				} else if str2ArrayReg.MatchString(colAttr.att_type) {
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
				} else if boolReg.MatchString(colAttr.att_type) {
					outputf(fmt.Sprintf("%sP_%s=%v,", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strutil.StoBol(att_value)))
				} else if boolArrayReg.MatchString(colAttr.att_type) {
					if v, err := strutil.ParseBools(strings.Split(att_value, ARRAY_SEPARATOR)); err != nil {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:%v", colAttr, err))
					} else {
						outputf(fmt.Sprintf("%sP_%s={%v},", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name, strings.Join(strutil.BoolsToStrs(v), ",")))
					}
				} else if bool2ArrayReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumVNumReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumVNumMapReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumV2NumMapReg.MatchString(colAttr.att_type) {

					//TODO ...
				} else if baseKNumVFloatReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumVFloatMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[int64]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k int64
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKNumV2FloatMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKNumVBoolReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumVBoolMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[int64]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k int64
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKNumV2BoolMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKNumVStrReg.MatchString(colAttr.att_type) {
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
				} else if baseKNumVStrMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[int64]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k int64
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKNumV2StrMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKFloatVNumReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatVNumMapReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatV2NumMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKFloatVFloatReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatVFloatMapReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatV2FloatMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKFloatVBoolReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatVBoolMapReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatV2BoolMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKFloatVStrReg.MatchString(colAttr.att_type) {
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
				} else if baseKFloatVStrMapReg.MatchString(colAttr.att_type) {
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
							value := strings.TrimSpace(kv[1])
							value = value[1 : len(value)-1]
							outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
						}
						outputf(fmt.Sprintf("\n%s},", colAttr.indent))
					} else {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
				} else if baseKFloatV2StrMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKBoolVNumReg.MatchString(colAttr.att_type) {
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
				} else if baseKBoolVNumMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[bool]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k bool
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKBoolV2NumMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKBoolVFloatReg.MatchString(colAttr.att_type) {
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
				} else if baseKBoolVFloatMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[bool]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k bool
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKBoolV2FloatMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKBoolVBoolReg.MatchString(colAttr.att_type) {
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
				} else if baseKBoolVBoolMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[bool]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k bool
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKBoolV2BoolMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKBoolVStrReg.MatchString(colAttr.att_type) {
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
				} else if baseKBoolVStrMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[bool]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k bool
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
				} else if baseKBoolV2StrMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKStrVNumReg.MatchString(colAttr.att_type) {
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

						if mainKeyReg.MatchString(k) { //字符串类型
							outputf(fmt.Sprintf(`%s%v=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else if baseKStrVNumMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[string]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k string
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
								if mainKeyReg.MatchString(k) { //字符串类型
									outputf(fmt.Sprintf(`%s%v={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
								} else {
									outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Int64sToStrs(v), ",")))
								}
							}
						}
						outputf(fmt.Sprintf("\n%s},", colAttr.indent))
					} else {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
				} else if baseKStrV2NumMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKStrVFloatReg.MatchString(colAttr.att_type) {
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

						if mainKeyReg.MatchString(k) { //字符串类型
							outputf(fmt.Sprintf(`%s%v=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else if baseKStrVFloatMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[string]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k string
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
								if mainKeyReg.MatchString(k) { //字符串类型
									outputf(fmt.Sprintf(`%s%v={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
								} else {
									outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.Float64sToStrs(v), ",")))
								}
							}
						}
						outputf(fmt.Sprintf("\n%s},", colAttr.indent))
					} else {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
				} else if baseKStrV2FloatMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKStrVBoolReg.MatchString(colAttr.att_type) {
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

						if mainKeyReg.MatchString(k) { //字符串类型
							outputf(fmt.Sprintf(`%s%v=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]=%v,`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else if baseKStrVBoolMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[string]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k string
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
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
								if mainKeyReg.MatchString(k) { //字符串类型
									outputf(fmt.Sprintf(`%s%v={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
								} else {
									outputf(fmt.Sprintf(`%s["%v"]={%v},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strutil.BoolsToStrs(v), ",")))
								}
							}
						}
						outputf(fmt.Sprintf("\n%s},", colAttr.indent))
					} else {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
				} else if baseKStrV2BoolMapReg.MatchString(colAttr.att_type) {
					//TODO ...
				} else if baseKStrVStrReg.MatchString(colAttr.att_type) {
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

						if mainKeyReg.MatchString(k) { //字符串类型
							outputf(fmt.Sprintf(`%s%v=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						} else {
							outputf(fmt.Sprintf(`%s["%v"]=[[%v]],`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, v))
						}
					}
					outputf(fmt.Sprintf("\n%s},", colAttr.indent))
				} else if baseKStrVStrMapReg.MatchString(colAttr.att_type) {
					if mapArrayValueReg.MatchString(att_value) {
						kvsm := mapArraySonValueReg.FindAllString(att_value, -1)
						result := make(map[string]bool)
						outputf(fmt.Sprintf("%sP_%s={", fmt.Sprintf("\n%s", colAttr.indent), colAttr.att_name))
						var k string
						for _, kvs := range kvsm {
							kv := strings.Split(kvs, MAP_SEPARATOR)
							if len(kv) != 2 {
								panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
							}
							k = strings.TrimSpace(kv[0])
							if ex := result[k]; ex {
								panic(fmt.Errorf("duplicate map key's value in field,loc:%+v", colAttr))
							}
							result[k] = true
							value := strings.TrimSpace(kv[1])
							value = value[1 : len(value)-1]
							if mainKeyReg.MatchString(k) { //字符串类型
								outputf(fmt.Sprintf(`%s%v={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
							} else {
								outputf(fmt.Sprintf(`%s["%v"]={[[%v]]},`, fmt.Sprintf("\n%s%s", colAttr.indent, INDENT), k, strings.Join(strings.Split(value, ARRAY_SEPARATOR), "]],[[")))
							}
						}
						outputf(fmt.Sprintf("\n%s},", colAttr.indent))
					} else {
						panic(fmt.Errorf("invalid type value,loc:%+v ,err:format error.", colAttr))
					}
				} else if baseKStrV2StrMapReg.MatchString(colAttr.att_type) {
					//TODO ...

				} else if objMapArrayReg.MatchString(colAttr.att_type) {

				} else { //TODO Map 结构的正则表达式处理 嵌套子节点的处理

				}
			}
		}
		outputf(fmt.Sprintf("%s},", fmt.Sprintf("\n%s", mk.indent)))
	}
}

func generateGoFromXLSXFile(xlsxFile *xlsx.File, sheetName string, outputf func(s string)) {
	sheet_root, ok := xlsxFile.Sheet[sheetName]
	if ok == false {
		panic(fmt.Errorf("No sheet %s available.\n", sheetName))
	}
	keyname, err := sheet_root.Rows[2].Cells[0].String()
	if err != nil {
		panic(err)
	}
	tmpl := template.Must(template.New("codeBaseTemplate").Parse(`
type SF_{{.Name}} map[int]*S_{{.Name}}

//获取模板数据(请勿在模板数据上修改数据)
func (f SF_{{.Name}}) Get(sid int) Sample {
	if s, pre := f[sid]; pre {
		return s
	}
	return nil
}

func (s *S_{{.Name}}) Sid() interface{} {
	return s.P_{{.MainKey}}
}

type S_{{.Name}} struct {
`))
	var bs bytes.Buffer
	if err := tmpl.Execute(&bs, struct {
		Name    string
		MainKey string
	}{sheetName, keyname}); err != nil {
		panic(err)
	}

	outputf(fmt.Sprintf("%s\n", bs.String()))
	hash := make(map[string]bool)
	for i, cell := range sheet_root.Rows[2].Cells {
		att_name, err := cell.String()
		if err != nil {
			panic(err)
		}
		att_name = strings.TrimSpace(att_name)
		if hash[att_name] {
			panic(fmt.Errorf(" sheet[%s] duplicate field name in struct literal: %s", sheetName, att_name))
		}
		hash[att_name] = true

		att_type, err := sheet_root.Rows[1].Cells[i].String()
		if err != nil {
			panic(err)
		}
		att_type = strings.TrimSpace(att_type)

		att_desc, err := sheet_root.Rows[0].Cells[i].String()
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
		} else if objMapArrayReg.MatchString(att_type) {
			son_sheetName := att_type
			base := ""
			if idx := strings.LastIndex(att_type, "]"); idx != -1 {
				son_sheetName = strings.TrimSpace(att_type[idx+1:])
				base = att_type[:idx+1]
			}
			if _, ok := sheetMap[son_sheetName]; !ok {
				sheetMap[son_sheetName] = true
				parseArray = append(parseArray, son_sheetName)
			}
			outputf(fmt.Sprintf("\tP_%s %sS_%s\n", att_name, base, son_sheetName))
		} else {
			panic(fmt.Errorf(`unknown struct defined "%s"`, att_type))
		}
	}
	outputf("}\n")
}

func generateGoMap(outputf func(s string)) {
	tmpl := template.Must(template.New("codeGoMapTemplate").Parse(`
		//Code generated by protoc-gen-go.
		//source: github.com/zxfonline/xlsx_parser
		//DO NOT EDIT!

		package sample

		import (
			"reflect"

			"github.com/zxfonline/golog"
		)

		var (
			_GLOBAL_MAP map[string]*sampleFactoryBuilder
			log         = golog.New("SampleFactory")
		)

		func init() {
			_GLOBAL_MAP = make(map[string]*sampleFactoryBuilder)
			//配置模板注册
			{{range .}}{{"\n"}}RegistSampleFactoryBuilder(&SF_{{.}}{}){{end}}
		}

		type sampleFactoryBuilder struct {
			typeOf reflect.Type
		}

		// only support pointer of a struct or a struct
		// &ta{} -> ta,ta{} -> ta
		// for debug use fmt.Printf("%T",xxx)
		func RegistSampleFactoryBuilder(factory SampleFactory) {
			typof := IndirectType(reflect.TypeOf(factory))
			name := typof.Name()
			//注册handler
			if _, pre := _GLOBAL_MAP[name]; pre {
				log.Infof("Replace sample builder \"%s\"", name)
			} else {
				log.Infof("Add sample builder \"%s\"", name)
			}
			_GLOBAL_MAP[name] = &sampleFactoryBuilder{typeOf: typof}
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

		func NewSampleFactory(name string) SampleFactory {
			if sample, pre := _GLOBAL_MAP[name]; pre {
				return reflect.New(sample.typeOf).Interface().(SampleFactory)
			} else {
				return nil
			}
		}

		//模板接口
		type Sample interface {
			Sid() interface{}
		}

		type SampleFactory interface {
			Get(sid int) Sample
		}
	`))
	var buf bytes.Buffer
	arr := make([]string, 0, len(sheetMap))
	for v, _ := range sheetMap {
		arr = append(arr, v)
	}
	err := tmpl.Execute(&buf, arr)
	if err != nil {
		panic(err)
	}
	outputf(buf.String())
}

//构建一个每日写日志文件的写入器
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
