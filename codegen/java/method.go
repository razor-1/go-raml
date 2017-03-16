package java

import (
	"fmt"
	"strings"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/codegen/resource"
	// "github.com/Jumpscale/go-raml/codegen/security"
	"github.com/Jumpscale/go-raml/raml"
	// log "github.com/Sirupsen/logrus"
)

// java server method
type serverMethod struct {
	*resource.Method
// 	MiddlewaresArr []middleware
}

// setup sets all needed variables

func (sm *serverMethod) setup(apiDef *raml.APIDefinition, r *raml.Resource, rd *resource.Resource) error {
	// method name
	if len(sm.DisplayName) > 0 {
		sm.MethodName = commons.DisplayNameToFuncName(sm.DisplayName)
	} else {
		// sm.MethodName = snakeCaseResourceURI(r) + "_" + strings.ToLower(sm.Verb())
	}
	sm.Params = strings.Join(resource.GetResourceParams(r), ", ")
	sm.Endpoint = strings.Replace(sm.Endpoint, "{", "<", -1)
	sm.Endpoint = strings.Replace(sm.Endpoint, "}", ">", -1)

	// security middlewares
	/*
	for _, v := range sm.SecuredBy {
		if !security.ValidateScheme(v.Name, apiDef) {
			continue
		}
		// oauth2 middleware
		m, err := newPythonOauth2Middleware(v)
		if err != nil {
			log.Errorf("error creating middleware for method.err = %v", err)
			return err
		}
		sm.MiddlewaresArr = append(sm.MiddlewaresArr, m)
	}
	*/
	return nil
}

// defines a java client lib method
type clientMethod struct {
	resource.Method
	PRArgs string // java requests's args
	PRCall string // the way we call java request
}

func newClientMethod(r *raml.Resource, rd *resource.Resource, m *raml.Method, methodName string) (resource.MethodInterface, error) {
	method := resource.NewMethod(r, rd, m, methodName, setBodyName)

	method.ResourcePath = commons.ParamizingURI(method.Endpoint, "+")

	name := commons.NormalizeURITitle(method.Endpoint)

	method.ReqBody = setBodyName(m.Bodies, name+methodName, "ReqBody")

	jcm := clientMethod{Method: method}
	jcm.setup()
	return jcm, nil
}

func (jcm *clientMethod) setup() {
	prArgs := []string{"uri"}  // prArgs are arguments we supply to java request
	params := []string{"self"} // params are method signature params

	// for method with request body, we add `data` argument
	if jcm.Verb() == "PUT" || jcm.Verb() == "POST" || jcm.Verb() == "PATCH" {
		params = append(params, "data")
		prArgs = append(prArgs, "data")
	}

	// construct prArgs string from the array
	prArgs = append(prArgs, "headers=headers", "params=query_params")
	jcm.PRArgs = strings.Join(prArgs, ", ")

	// construct method signature
	params = append(params, resource.GetResourceParams(jcm.Resource())...)
	params = append(params, "headers=None", "query_params=None")
	jcm.Params = strings.Join(params, ", ")

	// java request call
	// we encapsulate the call to put, post, and patch.
	// To be able to accept plain string or dict.
	// if it is a dict, we encode it to json
	if jcm.Verb() == "PUT" || jcm.Verb() == "POST" || jcm.Verb() == "PATCH" {
		jcm.PRCall = fmt.Sprintf("self.client.%v", strings.ToLower(jcm.Verb()))
	} else {
		jcm.PRCall = fmt.Sprintf("self.client.session.%v", strings.ToLower(jcm.Verb()))
	}

	if len(jcm.DisplayName) > 0 {
		jcm.MethodName = commons.DisplayNameToFuncName(jcm.DisplayName)
	} else {
		// jcm.MethodName = snakeCaseResourceURI(jcm.Resource()) + "_" + strings.ToLower(jcm.Verb())
	}
}

// create server resource's method

func newServerMethod(apiDef *raml.APIDefinition, r *raml.Resource, rd *resource.Resource, m *raml.Method,
	methodName string) resource.MethodInterface {

	method := resource.NewMethod(r, rd, m, methodName, setBodyName)
	// method.SecuredBy = security.GetMethodSecuredBy(apiDef, r, m)

	jm := serverMethod{
		Method: &method,
	}
	jm.setup(apiDef, r, rd)
	return jm
}

// create snake case function name from a resource URI
// func snakeCaseResourceURI(r *raml.Resource) string {
// 	return _snakeCaseResourceURI(r, "")
// }

// func _snakeCaseResourceURI(r *raml.Resource, completeURI string) string {
// 	if r == nil {
// 		return completeURI
// 	}
// 	var snake string
// 	if len(r.URI) > 0 {
// 		uri := commons.NormalizeURI(r.URI)
// 		if r.Parent != nil { // not root resource, need to add "_"
// 			snake = "_"
// 		}

// 		if strings.HasPrefix(r.URI, "/{") {
// 			snake += "by" + strings.ToUpper(uri[:1])
// 		} else {
// 			snake += strings.ToLower(uri[:1])
// 		}

// 		if len(uri) > 1 { // append with the rest of uri
// 			snake += uri[1:]
// 		}
// 	}
// 	return _snakeCaseResourceURI(r.Parent, snake+completeURI)
// }

// setBodyName set name of method's request/response body.
//
// Rules:
//  - use bodies.Type if not empty and not `object`
//  - use bodies.ApplicationJSON.Type if not empty and not `object`
//  - use prefix+suffix if:
//      - not meet previous rules
//      - previous rules produces JSON string
func setBodyName(bodies raml.Bodies, prefix, suffix string) string {
	var tipe string
	prefix = commons.NormalizeURITitle(prefix)

	if len(bodies.Type) > 0 && bodies.Type != "object" {
		tipe = bodies.Type
	} else if bodies.ApplicationJSON != nil {
		if bodies.ApplicationJSON.Type != "" && bodies.ApplicationJSON.Type != "object" {
			tipe = bodies.ApplicationJSON.Type
		} else {
			tipe = prefix + suffix
		}
	}

	if commons.IsJSONString(tipe) {
		tipe = prefix + suffix
	}

	return tipe

}
