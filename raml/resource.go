package raml

import (
	"fmt"
	"log"
	"path"
	"strings"
)

// A Resource is the conceptual mapping to an entity or set of entities.
type Resource struct {

	// Resources are identified by their relative URI, which MUST begin with
	// a slash (/).
	URI string

	// An alternate, human-friendly name for the resource.
	// If the displayName property is not defined for a resource,
	// documentation tools SHOULD refer to the resource by its property key
	// which acts as the resource name. For example, tools should refer to the relative URI /jobs.
	DisplayName string `yaml:"displayName"`

	// A substantial, human-friendly description of a resource.
	// Its value is a string and MAY be formatted using markdown.
	Description string `yaml:"description"`

	// TODO : annotationName

	// In a RESTful API, methods are operations that are performed on a
	// resource. A method MUST be one of the HTTP methods defined in the
	// HTTP version 1.1 specification [RFC2616] and its extension,
	// RFC5789 [RFC5789].
	Get     *Method `yaml:"get"`
	Patch   *Method `yaml:"patch"`
	Put     *Method `yaml:"put"`
	Head    *Method `yaml:"head"`
	Post    *Method `yaml:"post"`
	Delete  *Method `yaml:"delete"`
	Options *Method `yaml:"options"`

	// A list of traits to apply to all methods declared (implicitly or explicitly) for this resource.
	// Individual methods can override this declaration.
	Is []DefinitionChoice `yaml:"is"`

	// The resource type that this resource inherits.
	Type *DefinitionChoice `yaml:"type"`

	// The security schemes that apply to all methods declared (implicitly or explicitly) for this resource.
	SecuredBy []DefinitionChoice `yaml:"securedBy"`

	// Detailed information about any URI parameters of this resource.
	URIParameters map[string]NamedParameter `yaml:"uriParameters"`

	// A nested resource, which is identified as any property
	// whose name begins with a slash ("/"), and is therefore treated as a relative URI.
	Nested map[string]*Resource `yaml:",regexp:/.*"`

	// A resource defined as a child property of another resource is called a
	// nested resource, and its property's key is its URI relative to its
	// parent resource's URI. If this is not nil, then this resource is a
	// child resource.
	Parent *Resource

	// all methods of this resource
	Methods []*Method `yaml:"-"`
}

// postProcess doing post processing of a resource after being constructed by the parser.
// some of the workds:
// - assign all properties that can't be obtained from RAML document
// - inherit from resource type
// - inherit from traits
func (r *Resource) postProcess(uri string, parent *Resource, resourceTypes map[string]ResourceType, traitsMap map[string]Trait) error {
	r.URI = strings.TrimSpace(uri)
	r.Parent = parent

	r.setMethods(traitsMap)

	// inherit from resource types
	if err := r.inheritResourceType(resourceTypes); err != nil {
		return err
	}

	// process nested/child resources
	for k := range r.Nested {
		n := r.Nested[k]
		if err := n.postProcess(k, r, resourceTypes, traitsMap); err != nil {
			return err
		}
		r.Nested[k] = n
	}
	return nil
}

// inherit from a resource type
func (r *Resource) inheritResourceType(resourceTypes map[string]ResourceType) error {
	// get resource type object to inherit
	rt, err := r.getResourceType(resourceTypes)
	if rt == nil || err != nil {
		return err
	}

	// initialize dicts
	dicts := initResourceTypeDicts(r, r.Type.Parameters)

	r.Description = substituteParams(r.Description, rt.Description, dicts)

	// uri parameters
	if len(r.URIParameters) == 0 {
		r.URIParameters = map[string]NamedParameter{}
	}
	for name, up := range rt.URIParameters {
		p, ok := r.URIParameters[name]
		if !ok {
			p = NamedParameter{}
		}
		p.inherit(up, dicts)
		r.URIParameters[name] = p
	}

	// methods
	r.inheritMethods(rt)

	return nil
}

// inherit methods inherits all methods based on it's resource type
func (r *Resource) inheritMethods(rt *ResourceType) {
	// inherit all methods from resource type
	// if it doesn't have the methods, we create it
	for _, rtm := range rt.methods {
		m := r.MethodByName(rtm.Name)
		if m == nil {
			m = newMethod(rtm.Name)
			r.assignMethod(m, m.Name)
		}
		m.inheritFromResourceType(r, rtm, rt)
	}

	// inherit optional methods if only the resource also has the method
	for _, rtm := range rt.optionalMethods {
		m := r.MethodByName(rtm.Name)
		if m == nil {
			continue
		}
		m.inheritFromResourceType(r, rtm, rt)
	}

}

// get resource type from which this resource will inherit
func (r *Resource) getResourceType(resourceTypes map[string]ResourceType) (*ResourceType, error) {
	// check if it's specify a resource type to inherit
	if r.Type == nil || r.Type.Name == "" {
		return nil, nil
	}

	// get resource type from array of resource type map
	for k, rt := range resourceTypes {
		if k == r.Type.Name {
			return &rt, nil
		}
	}
	return nil, fmt.Errorf("can't find resource type named :%v", r.Type.Name)
}

// set methods set all methods name
// and add it to Methods slice
func (r *Resource) setMethods(traitsMap map[string]Trait) {
	if r.Get != nil {
		r.Get.postProcess(r, "GET", traitsMap)
	}
	if r.Post != nil {
		r.Post.postProcess(r, "POST", traitsMap)
	}
	if r.Put != nil {
		r.Put.postProcess(r, "PUT", traitsMap)
	}
	if r.Patch != nil {
		r.Patch.postProcess(r, "PATCH", traitsMap)
	}
	if r.Head != nil {
		r.Head.postProcess(r, "HEAD", traitsMap)
	}
	if r.Delete != nil {
		r.Delete.postProcess(r, "DELETE", traitsMap)
	}
	if r.Options != nil {
		r.Options.postProcess(r, "OPTIONS", traitsMap)
	}
}

// MethodByName return resource's method by it's name
func (r *Resource) MethodByName(name string) *Method {
	switch name {
	case "GET":
		return r.Get
	case "POST":
		return r.Post
	case "PUT":
		return r.Put
	case "PATCH":
		return r.Patch
	case "HEAD":
		return r.Head
	case "DELETE":
		return r.Delete
	case "OPTIONS":
		return r.Options
	default:
		return nil
	}
}

func (r *Resource) assignMethod(m *Method, name string) {
	switch name {
	case "GET":
		r.Get = m
	case "POST":
		r.Post = m
	case "PUT":
		r.Put = m
	case "PATCH":
		r.Patch = m
	case "HEAD":
		r.Head = m
	case "DELETE":
		r.Delete = m
	case "OPTIONS":
		r.Options = m
	default:
		log.Fatalf("assignMethod fatal error, invalid method name:%v", name)
	}
}

// substituteParams substitute all params inside double chevron to the correct value
// param value will be obtained from dicts map
func substituteParams(toReplace, words string, dicts map[string]interface{}) string {
	// non empty scalar node remain unchanged
	// except it has double chevron bracket
	if toReplace != "" && (strings.Index(toReplace, "<<") < 0 && strings.Index(toReplace, ">>") < 0) {
		return toReplace
	}
	if words == "" {
		return toReplace
	}

	removeParamBracket := func(param string) string {
		param = strings.TrimSpace(param)
		return param[2 : len(param)-2]
	}

	// search params
	params := dcRe.FindAllString(words, -1)

	// substitute the params
	for _, p := range params {
		pVal := getParamValue(removeParamBracket(p), dicts)
		words = strings.Replace(words, p, pVal, -1)
	}
	return words
}

// get value of a resource type param
func getParamValue(param string, dicts map[string]interface{}) string {
	// split between inflector and real param
	// real param and inflector is seperated by `|`
	cleanParam, inflector := func() (string, string) {
		arr := strings.Split(param, "|")
		if len(arr) != 2 {
			return param, ""
		}
		return strings.TrimSpace(arr[0]), strings.TrimSpace(arr[1])
	}()

	// get the value
	val := func() string {
		// get from type parameters
		val, ok := dicts[cleanParam]
		if !ok {
			log.Fatalf("getParamValue unknown param:%v, dicts=%v", cleanParam, dicts)
		}
		return fmt.Sprintf("%v", val)
	}()

	// inflect the value if needed
	if inflector != "" {
		var ok bool
		val, ok = doInflect(val, inflector)
		if !ok {
			log.Fatalf("invalid inflector " + inflector)
		}
	}
	return val
}

// CleanURI returns URI without `/`, `\`', `{`, and `}`
func (r *Resource) CleanURI() string {
	s := strings.Replace(r.URI, "/", "", -1)
	s = strings.Replace(s, `\`, "", -1)
	s = strings.Replace(s, "{", "", -1)
	s = strings.Replace(s, "}", "", -1)
	return strings.TrimSpace(s)
}

// FullURI returns full/absolute URI of this resource
func (r *Resource) FullURI() string {
	return doFullURI(r, "")
}

func doFullURI(r *Resource, completeURI string) string {
	completeURI = path.Join(r.URI, completeURI, "/")
	if r.Parent == nil {
		return completeURI
	}
	return doFullURI(r.Parent, completeURI)
}
