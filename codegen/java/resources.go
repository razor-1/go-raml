package java

import (
	"sort"
	"strings"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/codegen/resource"
	"github.com/Jumpscale/go-raml/raml"
)

const (
	resourcePyTemplate = "./templates/java_server_resource.tmpl"
)

type javaResource struct {
	*resource.Resource
// 	MiddlewaresArr []middleware
}

// func (jr *javaResource) addMiddleware(mwr middleware) {
// 	// check if already exist
// 	for _, v := range jr.MiddlewaresArr {
// 		if v.Name == mwr.Name {
// 			return
// 		}
// 	}
// 	jr.MiddlewaresArr = append(jr.MiddlewaresArr, mwr)
// }

func newResource(name string, apiDef *raml.APIDefinition, isServer bool) javaResource {
	rd := resource.New(apiDef, name, "")
	rd.IsServer = isServer
	r := javaResource{
		Resource: &rd,
	}
	res := apiDef.Resources[name]
	r.GenerateMethods(&res, "java", newServerMethod, newClientMethod)
	return r
}

// set middlewares to import
// func (jr *javaResource) setMiddlewares() {
// 	for _, v := range jr.Methods {
// 		pm := v.(serverMethod)
// 		for _, m := range pm.MiddlewaresArr {
// 			jr.addMiddleware(m)
// 		}
// 	}
// }

// generate flask rejresentation of an RAML resource
// It has one file : an API route and implementation
func (jr *javaResource) generate(r *raml.Resource, URI, dir string) error {
	jr.GenerateMethods(r, "java", newServerMethod, newClientMethod)
	// jr.setMiddlewares()
	filename := dir + "/" + strings.ToLower(jr.Name) + ".java"
	return commons.GenerateFile(jr, resourcePyTemplate, "resource_java_template", filename, true)
}

// return array of request body in this resource
func (jr javaResource) ReqBodies() []string {
	var reqs []string
	for _, m := range jr.Methods {
		pm := m.(serverMethod)
		if pm.ReqBody != "" && !commons.IsStrInArray(reqs, pm.ReqBody) {
			reqs = append(reqs, pm.ReqBody)
		}
	}
	sort.Strings(reqs)
	return reqs
}

func getAllResources(apiDef *raml.APIDefinition, isServer bool) []javaResource {
	rs := []javaResource{}

	// sort the keys, so we have resource sorted by keys.
	// the generated code actually don't need it to be sorted.
	// but test fixture need it
	var keys []string
	for k := range apiDef.Resources {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		rs = append(rs, newResource(k, apiDef, isServer))
	}
	return rs
}
