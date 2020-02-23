package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	options "google.golang.org/genproto/googleapis/api/annotations"
)

const messageTemplateString = `class {{ .Type.Name }} {{ if .Extends }}extends {{ .Extends }} {{ end }}{
    {{ range .Type.Field }}public {{ propertyType . $.PackageName }} {{ .Name }} { get; set; }
    {{ end }}
    {{ range .Type.NestedType }}class {{ .Name }} {
        {{ range .Field }}public {{ propertyType . $.PackageName }} {{ .Name }} { get; set; }
        {{ end }}
    }
    {{ end }}
}`

const serviceTemplateString = `class {{ .Name }}Service {{ if .Extends }}extends {{ .Extends }} {{ end }}{
    {{ range .Methods }}
    public {{ .OutputType }} {{ .Name }}({{ .InputType }} input) {
        String res = this.call('{{ .HttpMethod }}', '{{ .Path }}', JSON.serialize(input);
        return JSON.deserializeStrict(res.getBody(), {{ .OutputType }}.class);
    }
    {{ end }}
    private String call(String method, String path, String requestBody) {
        HttpRequest req = new HttpRequest();
        req.setMethod(method);

        // Set Callout timeout
        // default: 10 secs(that often causes "System.CalloutException: Read timed out")
        req.setTimeout(60000);

        // Set HTTPRequest header properties
        req.setEndpoint('{{ .EndpointBase }}' + path);
        req.setBody(requestBody);

        Http http = new Http();
        HTTPResponse res;
        String content;
        // Execute Http Callout
        res = http.send(req);

        if (res.getStatusCode() == 401) {
            // throw new Exception(res.getStatus());
        }
        return res;
    }
}`

type templateBind struct {
	Type        *descriptor.DescriptorProto
	PackageName string
	Extends     string
}

type method struct {
	Name       string
	HttpMethod string
	Path       string
	InputType  string
	OutputType string
}

type clientBind struct {
	Name         string
	Methods      []*method
	Extends      string
	EndpointBase string
}

var (
	messageTemplate *template.Template
	serviceTemplate *template.Template
	extendsMessage  string
	extendsService  string
)

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
	for _, p := range strings.Split(req.GetParameter(), ",") {
		spec := strings.SplitN(p, "=", 2)
		if len(spec) == 1 {
			continue
		}
		name, value := spec[0], spec[1]
		if name == "extends_message" {
			extendsMessage = value
		} else if name == "extends_service" {
			extendsService = value
		}
	}

	files := make(map[string]*descriptor.FileDescriptorProto)
	for _, f := range req.ProtoFile {
		files[f.GetName()] = f
	}
	var resp plugin.CodeGeneratorResponse
	for _, fname := range req.FileToGenerate {
		f := files[fname]
		for _, m := range f.MessageType {
			out := m.GetName() + ".cls"
			b := bytes.NewBuffer([]byte{})
			err := messageTemplate.Execute(b, templateBind{Type: m, PackageName: f.GetPackage(), Extends: extendsMessage})
			if err != nil {
				panic(err)
			}
			resp.File = append(resp.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(out),
				Content: proto.String(b.String()),
			})
		}

		for _, sv := range f.GetService() {
			var methods []*method
			for _, m := range sv.GetMethod() {
				if !proto.HasExtension(m.Options, options.E_Http) {
					continue
				}
				ext, err := proto.GetExtension(m.Options, options.E_Http)
				if err != nil {
					panic(err)
				}
				opts, ok := ext.(*options.HttpRule)
				if !ok {
					panic("no http rule")
				}
				httpMethod, path := requestInfo(opts)
				methods = append(methods, &method{
					Name:       m.GetName(),
					Path:       path,
					InputType:  strings.Replace(m.GetInputType(), "."+f.GetPackage()+".", "", -1),
					OutputType: strings.Replace(m.GetOutputType(), "."+f.GetPackage()+".", "", -1),
					HttpMethod: httpMethod,
				})
			}
			out := sv.GetName() + "Service.cls"
			b := bytes.NewBuffer([]byte{})
			err := serviceTemplate.Execute(b, clientBind{
				Name:         sv.GetName(),
				Methods:      methods,
				EndpointBase: "https://example.com",
				Extends:      extendsService,
			})
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

func requestInfo(o *options.HttpRule) (string, string) {
	if o.GetGet() != "" {
		return "GET", o.GetGet()
	} else if o.GetPost() != "" {
		return "POST", o.GetPost()
	} else if o.GetPatch() != "" {
		return "PATCH", o.GetPatch()
	} else if o.GetPut() != "" {
		return "PUT", o.GetPut()
	} else if o.GetDelete() != "" {
		return "DELETE", o.GetDelete()
	}
	panic(o)
}

func emitResp(resp *plugin.CodeGeneratorResponse) error {
	buf, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(buf)
	return err
}

func getType(f *descriptor.FieldDescriptorProto, packageName string) string {
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
}

func initTemplate() {
	var err error
	messageTemplate, err = template.New("apex").Funcs(template.FuncMap{
		"propertyType": func(f *descriptor.FieldDescriptorProto, packageName string) string {
			format := "%s"
			if f.GetLabel() == descriptor.FieldDescriptorProto_LABEL_REPEATED {
				format = "List<%s>"
			}
			return fmt.Sprintf(format, getType(f, packageName))
		},
	}).Parse(messageTemplateString)
	if err != nil {
		panic(err)
	}
	serviceTemplate, err = template.New("service").Parse(serviceTemplateString)
	if err != nil {
		panic(err)
	}
}

func run() error {
	initTemplate()
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
