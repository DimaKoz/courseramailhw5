package main

/*
there is a working solution of the 1st week
of programming assignment of 'Golang Webservices 2' course
https://www.coursera.org/learn/golang-webservices-2
*/

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"sort"
	"strings"
	"text/template"
)

type FieldType int

const (
	FieldTypeUnknown FieldType = iota
	FieldTypeInt
	FieldTypeString
)

type ValidatorAction string

const (
	ValidatorUndef     ValidatorAction = ""
	ValidatorRequired  ValidatorAction = "required"
	ValidatorParamName ValidatorAction = "paramname"
	ValidatorEnum      ValidatorAction = "enum"
	ValidatorDefault   ValidatorAction = "default"
	ValidatorMin       ValidatorAction = "min"
	ValidatorMax       ValidatorAction = "max"
)

//an implementation of 'Stringer' interface
func (v ValidatorAction) String() string {
	return string(v)
}

const (
	PosRequired = iota
	PosParamName
	PosMin
	PosMax
	PosDefault
	PosEnum
)

//Order gets an order for sorting
func (v ValidatorAction) Order() int {
	switch v {
	case ValidatorDefault:
		return PosDefault
	case ValidatorEnum:
		return PosEnum
	case ValidatorParamName:
		return PosParamName
	case ValidatorMax:
		return PosMax
	case ValidatorMin:
		return PosMin
	case ValidatorRequired:
		return PosRequired
	default:
		return PosRequired
	}
}

type ConditionString struct {
	Key   ValidatorAction
	Value string
}

type FieldDesc struct {
	Name             string
	Type             FieldType
	ConditionsString []ConditionString
}

type StructDesc struct {
	Name   string
	fields []FieldDesc
}

type ApiDesc struct {
	structs  map[string]StructDesc
	handlers []Handler
}

type HandlerApiGenComment struct {
	Url    string
	Auth   bool
	Method string
}

type Handler struct {
	StructName    string
	Meta          HandlerApiGenComment
	HandlerMethod string
	ParamIn       string
	ResultOut     string
	ParamInStruct []StructDesc
}

var funcMap = template.FuncMap{
	"CapitalizeFirst": strings.Title,
}

var structRespTpl = template.Must(template.New("structTpl").Funcs(funcMap).Parse(`
type Resp{{ .StructName }}{{ .HandlerMethod | CapitalizeFirst }}  struct {
	{{ .ResultOut }}` + " `json:\"response\"`\n" +
	" EmptyError string `json:\"error\"` \n}\n\n"))

func main() {

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}
	var buffer bytes.Buffer
	out, _ := os.Create(os.Args[2])
	defer func() {
		_ = out.Close()
		fmt.Println("Done")
	}()

	generateImportSection(&buffer, node.Name.Name)
	generateApiErrorsFuncSection(&buffer)
	generateApiErrorsSection(&buffer)
	generateAuxiliaryFunctionsSection(&buffer)

	apiDesc := ApiDesc{
		structs:  make(map[string]StructDesc),
		handlers: make([]Handler, 0),
	}
	for _, f := range node.Decls {
		switch a := f.(type) {
		case *ast.GenDecl:
			gatherInfoStruct(a, &apiDesc)
		case *ast.FuncDecl:
			gatherInfoFunc(a, &apiDesc)
		default:
			continue
		}
	}

	generateResponseStructSection(&buffer, apiDesc.handlers)
	generateHandlers(&buffer, apiDesc.handlers, apiDesc.structs)
	generateServeFunc(&buffer, apiDesc.handlers)
	formatCode(&buffer)

	//saving
	_, _ = fmt.Fprintln(out, buffer.String())

}

//generateHandlers generates handlers
func generateHandlers(b *bytes.Buffer, handlers []Handler, structs map[string]StructDesc) {
	fmt.Println("Generating handlers")
	//only stubs for now

	for _, h := range handlers {
		str := `
	func (srv *` + h.StructName + `) ` + h.HandlerMethod + `  (w http.ResponseWriter, r *http.Request){` +
			"\n\n"
		b.WriteString(str)
		//Post or not
		var paramInStruct StructDesc
		var foundParamInStruct = false
		if h.Meta.Method == "POST" {
			str := `			if r.Method != "POST" {
				errBadMethod.serve(w)
				return
			}
`
			b.WriteString(str)
			if h.Meta.Auth {
				str = `
				if r.Header.Get("X-Auth") != "100500" {
				errUnauthorized.serve(w)
				return
				}`
				b.WriteString(str)
			}

			b.WriteString("\n_ = r.ParseForm()\n")

			if paramInStruct, foundParamInStruct = structs[h.ParamIn]; foundParamInStruct {
				for _, field := range paramInStruct.fields {
					lowFieldName := strings.ToLower(field.Name)
					if field.Type == FieldTypeInt {
						str = lowFieldName + ", err := strconv.Atoi(r.Form.Get(\"" + lowFieldName + "\"))\n"
						str += `if err != nil {
							NewApiError("` + lowFieldName + ` must be int", http.StatusBadRequest).serve(w)
							return
						}`
					} else {
						str = lowFieldName + " := r.Form.Get(\"" + lowFieldName + "\")\n"
					}
					b.WriteString(str + "\n")

					for _, cond := range field.ConditionsString {
						if cond.Key == ValidatorRequired {
							if field.Type == FieldTypeInt {
								str = `if ` + lowFieldName + ` == nil {
								errEmptyLogin.serve(w)
								return
							}`
							} else {
								str = `if ` + lowFieldName + ` == "" {
								errEmptyLogin.serve(w)
								return
							}`
							}
							b.WriteString(str + "\n")
						}
						if cond.Key == ValidatorMin {
							if field.Type == FieldTypeInt {
								str = `if ` + lowFieldName + ` < ` + cond.Value + ` {
									NewApiError("` + lowFieldName + ` must be >= ` + cond.Value + `", http.StatusBadRequest).serve(w)
									return
								}`
							} else {
								str = `if len(` + lowFieldName + `) < ` + cond.Value + ` {
									NewApiError("` + lowFieldName + ` len must be >= ` + cond.Value + `", http.StatusBadRequest).serve(w)
									return
								}`
							}
							b.WriteString(str + "\n")
						}
						if cond.Key == ValidatorMax {
							if field.Type == FieldTypeInt {
								str = `if ` + lowFieldName + ` > ` + cond.Value + ` {
									NewApiError("` + lowFieldName + ` must be <= ` + cond.Value + `", http.StatusBadRequest).serve(w)
									return
								}`
							} else {
								str = `if len(` + lowFieldName + `) > ` + cond.Value + ` {
									NewApiError("` + lowFieldName + ` len must be <= ` + cond.Value + `", http.StatusBadRequest).serve(w)
									return
								}`
							}
							b.WriteString(str + "\n")
						}
						if cond.Key == ValidatorParamName {
							str = cond.Value + ` := r.Form.Get("` + cond.Value + `")
							if ` + cond.Value + ` != "" {
								` + lowFieldName + ` = ` + cond.Value + `
							}`
							b.WriteString(str + "\n")
						}
						if cond.Key == ValidatorDefault {

							str = `if ` + lowFieldName + ` == "" {
								` + lowFieldName + ` = "` + cond.Value + `"
							}`
							b.WriteString(str + "\n")
						}
						if cond.Key == ValidatorEnum {
							condValues := strings.Split(cond.Value, "|")
							b.WriteString("m := make(map[string]bool)\n")
							for _, condV := range condValues {
								str = `m["` + condV + `"] = true`
								b.WriteString(str + "\n")
							}
							str = `_, prs := m[` + lowFieldName + `]
							if prs == false {
								NewApiError("` + lowFieldName + ` must be one of [` + strings.Join(condValues, ", ") + `]", http.StatusBadRequest).serve(w)
								return
							}`
							b.WriteString(str + "\n")
						}
					}

				}

			}
		} else {
			paramInStruct, foundParamInStruct = structs[h.ParamIn]
			for _, field := range paramInStruct.fields {
				lowFieldName := strings.ToLower(field.Name)
				str = `	var ` + lowFieldName + ` string

	switch r.Method {
	case "GET":
		` + lowFieldName + ` = r.URL.Query().Get("` + lowFieldName + `")
		if ` + lowFieldName + ` == "" {
			errEmptyLogin.serve(w)
			return
		}

	case "POST":
		_ = r.ParseForm()
		` + lowFieldName + ` = r.Form.Get("` + lowFieldName + `")
		if ` + lowFieldName + ` == "" {
			errEmptyLogin.serve(w)
			return
		}
	}
`
				str += "\n"
				b.WriteString(str)
			}
		}

		// Create a struct of parameters
		if foundParamInStruct {
			str = strings.ToLower(h.ParamIn) + ` := ` + h.ParamIn + "{\n"
			for _, field := range paramInStruct.fields {
				str += field.Name + ":" + strings.ToLower(field.Name) + ",\n"
			}
			str += "\n"
			b.WriteString(str)

			b.WriteString("\n}\n")

			str = strings.ToLower(h.ResultOut) + `, err := srv.` + strings.Title(h.HandlerMethod) + `(r.Context(), ` + strings.ToLower(h.ParamIn) + ")\n"
			b.WriteString(str)

			str = `	if err != nil {
		switch err.(type) {
		case ApiError:
			err.(ApiError).serve(w)
			return
		default:
			errBadUser.serve(w)
			return
		}
	}
`
			b.WriteString(str)

			str = `resp := Resp` + h.StructName + strings.Title(h.HandlerMethod) + `{
				` + h.ResultOut + `:    *` + strings.ToLower(h.ResultOut) + `,
				EmptyError: "",
			}
			serveAnswer(w, resp)`
			b.WriteString(str + "\n")
		}
		b.WriteString("\n}\n")
	}

}

//generateHandlers generates ServeHTTP functions
func generateServeFunc(b *bytes.Buffer, handlers []Handler) {
	fmt.Println("Generating 'ServeHTTP' functions")
	//sort handlers
	sorted := make(map[string][]Handler)
	for _, h := range handlers {
		if found, ok := sorted[h.StructName]; ok {
			found = append(found, h)
			sorted[h.StructName] = found
		} else {
			hCollection := make([]Handler, 0, len(handlers))
			hCollection = append(hCollection, h)
			sorted[h.StructName] = hCollection
		}
	}

	for k, v := range sorted {

		serveHandlerPart := "\nfunc (srv *" + k + ") ServeHTTP(w http.ResponseWriter, r *http.Request) { \n" +
			" switch r.URL.Path {" + "\n"

		for _, h := range v {
			serveHandlerPart += "case \"" + h.Meta.Url + "\":\n"
			serveHandlerPart += "srv." + h.HandlerMethod + "(w, r)\n"
		}
		serveHandlerPart += "default:\nerrUnknown.serve(w)\nreturn\n}} \n"
		b.WriteString(serveHandlerPart)

	}

}

//generateHandlers generates structures for responses
func generateResponseStructSection(buf *bytes.Buffer, handlers []Handler) {
	fmt.Println("Generating 'Responses Structures' Section")
	beginning := `
/*
"Responses Structures" Section
Beginning of "Responses Structures" section
*/
	`

	end := `/*
The end of "Responses Structures" section
*/`

	w := bufio.NewWriter(buf)
	_, _ = w.WriteString(beginning)
	for _, h := range handlers {
		if err := structRespTpl.Execute(w, h); err != nil {
			fmt.Println("Unexpected error: ", err.Error(), "while using handler: ", h)
			return
		}

	}
	_, _ = w.WriteString(end + "\n\n")
	if err := w.Flush(); err != nil {
		fmt.Println("Unexpected error in w.Flush(): ", err.Error())
	}
}

//gatherInfoStruct gathers information about structures
func gatherInfoStruct(a *ast.GenDecl, apiDesc *ApiDesc) {
	for _, spec := range a.Specs {
		currType, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		currStruct, ok := currType.Type.(*ast.StructType)
		if !ok {
			continue
		}
		gatherInfoFields(currStruct, currType, apiDesc)
	}
}

//gatherInfoFields gathers information about fields of structures
func gatherInfoFields(currStruct *ast.StructType, currType *ast.TypeSpec, apiDesc *ApiDesc) {
	var newStr *StructDesc = nil

	for _, field := range currStruct.Fields.List {
		if field.Tag != nil {
			tagValue := ""
			if strings.HasPrefix(field.Tag.Value, "`apivalidator:") {
				tagValue = strings.TrimLeft(field.Tag.Value, "`apivalidator:")
				if newStr == nil {
					if found, ok := apiDesc.structs[currType.Name.Name]; !ok {
						newStr = &StructDesc{Name: currType.Name.Name}
					} else {
						newStr = &found
					}
				}
			} else {
				continue
			}
			fType := FieldTypeUnknown
			if field.Type.(*ast.Ident).Name == "int" {
				fType = FieldTypeInt
			} else if field.Type.(*ast.Ident).Name == "string" {
				fType = FieldTypeString
			} else {
				continue
			}
			parseTagValue(newStr, field.Names[0].Name, fType, tagValue)
		}
		if newStr != nil && len(newStr.fields) > 0 {
			apiDesc.structs[currType.Name.Name] = *newStr
		}
	}
}

//parseTagValue parses values of 'apivalidator' tags
func parseTagValue(strDesc *StructDesc, fieldName string, fType FieldType, tagValue string) {
	conditions := strings.Split(strings.Trim(strings.Trim(tagValue, "/`"), "\""), ",")
	field := FieldDesc{Type: fType, Name: fieldName}
	for _, condition := range conditions {
		kv := strings.Split(condition, "=")
		if len(kv) == 1 { //one key only
			if kv[0] == ValidatorRequired.String() {
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorRequired})
			} else {
				fmt.Println("Unknown condition: ", condition)
				continue
			}
		} else if len(kv) == 2 { //a pair of key/value
			k, v := kv[0], kv[1]

			switch k {

			case ValidatorParamName.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorParamName, Value: v})
			case ValidatorEnum.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorEnum, Value: v})
			case ValidatorDefault.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorDefault, Value: v})
			case ValidatorMin.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorMin, Value: v})
			case ValidatorMax.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorMax, Value: v})
			case ValidatorRequired.String():
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorRequired, Value: v})
			default:
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorUndef, Value: v})

			}
		} else { // an unknown case
			fmt.Println("Unknown condition: ", condition)
			continue
		}
	}
	if len(field.ConditionsString) > 0 {
		sort.Slice(field.ConditionsString, func(i, j int) bool {
			return field.ConditionsString[i].Key.Order() < field.ConditionsString[j].Key.Order()
		})
		strDesc.fields = append(strDesc.fields, field)
	} else {
		fmt.Println("empty field: ", field)
	}
}

//gatherInfoFunc collects information with 'apigen:api' commentaries
func gatherInfoFunc(f *ast.FuncDecl, a *ApiDesc) {

	h := Handler{}

	if f.Doc == nil {
		return
	}
	for _, comment := range f.Doc.List {
		if strings.HasPrefix(comment.Text, "// apigen:api") {

			h.HandlerMethod = strings.ToLower(f.Name.Name)

			apigenDoc := []byte(strings.TrimLeft(comment.Text, "// apigen:api"))
			_ = json.Unmarshal(apigenDoc, &h.Meta)

			if f.Recv != nil {
				switch a := f.Recv.List[0].Type.(type) {
				case *ast.StarExpr:
					h.StructName = a.X.(*ast.Ident).Name
				}
			}

			if f.Type.Params.List != nil {
				for _, p := range f.Type.Params.List {
					switch a := p.Type.(type) {
					case *ast.Ident:
						h.ParamIn = a.Name
					}
				}
			}

			if f.Type.Results.List != nil && len(f.Type.Results.List) != 0 {
				switch a := f.Type.Results.List[0].Type.(type) {
				case *ast.StarExpr:
					h.ResultOut = a.X.(*ast.Ident).Name
				}
			}

			a.handlers = append(a.handlers, h)
		}
	}

}

//generateAuxiliaryFunctionsSection appends to *bytes.Buffer auxiliary functions
func generateAuxiliaryFunctionsSection(buf *bytes.Buffer) {
	fmt.Println("Generating auxiliary functions")
	errSection := `/*
"Auxiliary functions" section
Beginning of "Auxiliary functions" section
*/

func serveAnswer(w http.ResponseWriter, v interface{}) {
	data, _ := json.Marshal(v)
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
/*
The end of "Auxiliary functions" section
*/
`
	_, _ = buf.WriteString("\n" + errSection + "\n\n")
}

//generateApiErrorsSection appends to *bytes.Buffer Hardcoded Well-known Errors Section
func generateApiErrorsSection(buf *bytes.Buffer) {
	fmt.Println("Generating 'Hardcoded Well-known Errors' Section")
	errSection := `/*
"Hardcoded Well-known Errors" section
Beginning of "Hardcoded Well-known Errors" section
*/

var (
	errUnknown      = ApiError{HTTPStatus: http.StatusNotFound, Err: errors.New("unknown method")}
	errBadMethod    = ApiError{HTTPStatus: http.StatusNotAcceptable, Err: errors.New("bad method")}
	errEmptyLogin   = ApiError{HTTPStatus: http.StatusBadRequest, Err: errors.New("login must me not empty")}
	errBadUser      = ApiError{HTTPStatus: http.StatusInternalServerError, Err: errors.New("bad user")}
	errUnauthorized = ApiError{HTTPStatus: http.StatusForbidden, Err: errors.New("unauthorized")}
)
/*
The end of "Hardcoded Well-known Errors" section
*/
`
	_, _ = buf.WriteString("\n" + errSection + "\n\n")
}

//formatCode works like 'gofmt' command and formats a text in *bytes.Buffer
func formatCode(buf *bytes.Buffer) {
	fmt.Println("Formatting the code")
	b, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Println("Can't format: ", err.Error())
		return
	}
	buf.Reset()
	buf.WriteString(string(b))
}

//generateImportSection appends dependencies to *bytes.Buffer
func generateImportSection(buf *bytes.Buffer, packageName string) {
	fmt.Println("Generating 'Import' Section")
	imports := []string{
		"encoding/json",
		"errors",
		"fmt",
		"net/http",
		"strconv",
	}
	_, _ = buf.WriteString("package " + packageName + "\n")
	_, _ = buf.WriteString("import (\n")
	for _, impItem := range imports {
		_, _ = buf.WriteString("\"" + impItem + "\"\n")
	}
	_, _ = buf.WriteString(")\n\n")

}

//generateApiErrorsSection appends to *bytes.Buffer Hardcoded Well-known Errors Section
func generateApiErrorsFuncSection(buf *bytes.Buffer) {
	fmt.Println("Generating 'Functions Of ApiError' Section")
	_, _ = buf.WriteString("\n")
	errFuncSection := `
	/*
	"Functions Of ApiError" section -
	there are additional methods for well-known ApiError
	to eliminate some doubled code

	Beginning of "Functions Of ApiError" section
	*/

	func NewApiError(text string, httpStatus int) *ApiError {
		return &ApiError{HTTPStatus: httpStatus, Err: errors.New(text)}
	}

	func (ae ApiError) PrepApiAnswer() []byte {
		return []byte("{ \"error\":\"" + ae.Error() + "\"}")
	}

	func (ae ApiError) serve(w http.ResponseWriter) {
		apiData := ae.PrepApiAnswer()
		w.Header().Set("Content-Type", http.DetectContentType(apiData))
		w.WriteHeader(ae.HTTPStatus)
	_, err := w.Write(apiData)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
/*
The end of "functions of ApiError" section
*/

`
	_, _ = buf.WriteString("\n" + errFuncSection + "\n\n")

}
