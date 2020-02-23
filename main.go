package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

const templateString = `class {{ .Type.Name }} {{ if .Extends }}extends {{ .Extends }} {{ end }}{
    {{ range .Type.Field }}public {{ propertyType . $.PackageName }} {{ .Name }} { get; set; }
    {{ end }}
    {{ range .Type.NestedType }}class {{ .Name }} {
        {{ range .Field }}public {{ propertyType . $.PackageName }} {{ .Name }} { get; set; }
        {{ end }}
    }
    {{ end }}
}`

type templateBind struct {
	Type        *descriptor.DescriptorProto
	PackageName string
	Extends     string
}

func parseReq(r io.Reader) (*plugin.CodeGeneratorRequest, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var req plugin.CodeGeneratorRequest
	if err = proto.Unmarshal(buf, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func processReq(req *plugin.CodeGeneratorRequest) *plugin.CodeGeneratorResponse {
	files := make(map[string]*descriptor.FileDescriptorProto)
	for _, f := range req.ProtoFile {
		files[f.GetName()] = f
	}

	var extends string
	for _, p := range strings.Split(req.GetParameter(), ",") {
		spec := strings.SplitN(p, "=", 2)
		if len(spec) == 1 {
			continue
		}
		name, value := spec[0], spec[1]
		if name == "extends" {
			extends = value
		}
	}
	t, err := template.New("apex").Funcs(template.FuncMap{
		"propertyType": func(f *descriptor.FieldDescriptorProto, packageName string) string {
			switch f.GetType() {
			case descriptor.FieldDescriptorProto_TYPE_STRING:
				return "String"
			case descriptor.FieldDescriptorProto_TYPE_INT32,
				descriptor.FieldDescriptorProto_TYPE_INT64,
				descriptor.FieldDescriptorProto_TYPE_UINT32,
				descriptor.FieldDescriptorProto_TYPE_UINT64,
				descriptor.FieldDescriptorProto_TYPE_SINT32,
				descriptor.FieldDescriptorProto_TYPE_SINT64:
				return "Integer"
			case descriptor.FieldDescriptorProto_TYPE_BOOL:
				return "Boolean"
			case descriptor.FieldDescriptorProto_TYPE_DOUBLE,
				descriptor.FieldDescriptorProto_TYPE_FLOAT:
				return "Double"
			case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
				return strings.Replace(f.GetTypeName(), "."+packageName+".", "", -1)
			}
			return "unknown"
		},
	}).Parse(templateString)
	if err != nil {
		panic(err)
	}
	var resp plugin.CodeGeneratorResponse
	for _, fname := range req.FileToGenerate {
		f := files[fname]
		for _, m := range f.MessageType {
			out := *m.Name + ".cls"
			b := bytes.NewBuffer([]byte{})
			err := t.Execute(b, templateBind{Type: m, PackageName: f.GetPackage(), Extends: extends})
			if err != nil {
				panic(err)
			}
			resp.File = append(resp.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(out),
				Content: proto.String(b.String()),
			})
		}
	}
	return &resp
}

func emitResp(resp *plugin.CodeGeneratorResponse) error {
	buf, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(buf)
	return err
}

func run() error {
	req, err := parseReq(os.Stdin)
	if err != nil {
		return err
	}

	resp := processReq(req)

	return emitResp(resp)
}

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}
