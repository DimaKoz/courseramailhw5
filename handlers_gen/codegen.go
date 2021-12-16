package main

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
	"strconv"
	"strings"
	"text/template"
)

type FieldType int

const (
	FieldTypeUnknown FieldType = iota
	FieldTypeInt
	FieldTypeString
)

const (
	ValidatorRequired  = "required"
	ValidatorParamName = "paramname"
	ValidatorEnum      = "enum"
	ValidatorDefault   = "default"
	ValidatorMin       = "min"
	ValidatorMax       = "max"
)

type ConditionInt struct {
	Key   string
	Value int
}

type ConditionString struct {
	Key   string
	Value string
}

type FieldDesc struct {
	Name             string
	Type             FieldType
	ConditionsInt    []ConditionInt
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
	generateHandlerStubs(&buffer, apiDesc.handlers)
	generateServeFunc(&buffer, apiDesc.handlers)
	formatCode(&buffer)

	//saving
	fmt.Fprintln(out, buffer.String())

}

func generateHandlerStubs(b *bytes.Buffer, handlers []Handler) {
	//only stubs for now
	var stubHandlerTpl = template.Must(template.New("stubHandlerTpl").Parse(`
	func (srv *{{ .StructName }}) {{ .HandlerMethod }}  (w http.ResponseWriter, r *http.Request){
	}` + "\n\n"))
	w := bufio.NewWriter(b)
	for _, h := range handlers {
		if err := stubHandlerTpl.Execute(w, h); err != nil {
			fmt.Println("Unexpected error: ", err.Error(), "while using handler: ", h)
			return
		}

	}
	if err := w.Flush(); err != nil {
		fmt.Println("Unexpected error in w.Flush(): ", err.Error())
	}

}

func generateServeFunc(b *bytes.Buffer, handlers []Handler) {

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

		serveHandlerPart := "\nfunc (srv *" + k + ") ServeHTTP(w http.ResponseWriter, r *http.Request) { \n switch r.URL.Path {" + "\n"


		for _, h := range v {
			serveHandlerPart+="case \""+h.Meta.Url+"\":\n"
			serveHandlerPart+="srv."+h.HandlerMethod+"(w, r)\n"
		}
		serveHandlerPart+="default:\nerrUnknown.serve(w)\nreturn\n}} \n"
		b.WriteString(serveHandlerPart)

	}


}

func generateResponseStructSection(buf *bytes.Buffer, handlers []Handler) {
	fmt.Println("Generating 'Responses Structures' Section")
	beginning := `
/*
"Responses Structures" Section
Beginning of "Responses Structures" section
*/
	`

	end:= `/*
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
	_, _ = w.WriteString(end+"\n\n")
	if err := w.Flush(); err != nil {
		fmt.Println("Unexpected error in w.Flush(): ", err.Error())
	}
}

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

func parseTagValue(strDesc *StructDesc, fieldName string, fType FieldType, tagValue string) {
	conditions := strings.Split(strings.Trim(strings.Trim(tagValue, "/`"), "\""), ",")
	field := FieldDesc{Type: fType, Name: fieldName}
	for _, condition := range conditions {
		kv := strings.Split(condition, "=")
		if len(kv) == 1 { //one key only
			if kv[0] == ValidatorRequired {
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: ValidatorRequired})
			} else {
				fmt.Println("Unknown condition: ", condition)
				continue
			}
		} else if len(kv) == 2 { //a pair of key/value
			k, v := kv[0], kv[1]
			switch k {
			case ValidatorParamName:
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: k, Value: v})
			case ValidatorEnum:
				field.ConditionsString = append(field.ConditionsString, ConditionString{Key: k, Value: v})
			case ValidatorDefault:
				if fType == FieldTypeString {
					field.ConditionsString = append(field.ConditionsString, ConditionString{Key: k, Value: v})
				} else {
					if err := fillIntValue(&field, k, v); err != nil {
						fmt.Println("Unexpected condition: ", condition, "error: ", err.Error())
						continue
					}
				}
			case ValidatorMin:
				if err := fillIntValue(&field, k, v); err != nil {
					fmt.Println("Unexpected condition: ", condition, "error: ", err.Error())
					continue
				}
			case ValidatorMax:
				if err := fillIntValue(&field, k, v); err != nil {
					fmt.Println("Unexpected condition: ", condition, "error: ", err.Error())
					continue
				}

			}
		} else { // an unknown case
			fmt.Println("Unknown condition: ", condition)
			continue
		}
	}
	if len(field.ConditionsInt) > 0 || len(field.ConditionsString) > 0 {
		strDesc.fields = append(strDesc.fields, field)
	} else {
		fmt.Println("empty field: ", field)
	}
}

func fillIntValue(field *FieldDesc, k string, v string) error {
	vInt, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	field.ConditionsInt = append(field.ConditionsInt, ConditionInt{Key: k, Value: vInt})
	return nil
}

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
	bytes, _ := format.Source(buf.Bytes())
	buf.Reset()
	buf.WriteString(string(bytes))
}

//generateApiErrorsSection appends to *bytes.Buffer Hardcoded Well-known Errors Section
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
