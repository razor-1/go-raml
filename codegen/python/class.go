package python

import (
	"path/filepath"
	"sort"
	// "fmt"
	// "strings"

	// "github.com/chuckpreslar/inflect"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/raml"
)

// class defines a python class
type class struct {
	T           raml.Type
	Name        string
	Description []string
	Fields      map[string]field
	Enum        *enum
}

type objectProperty struct {
	name 			string
	required 		bool
	datatype 		string
	childProperties []objectProperty
}

// create a python class representations
func newClass(name string, T raml.Type, types map[string]raml.Type) class {
	pc := class{
		Name:        name,
		Description: commons.ParseDescription(T.Description),
		Fields:      map[string]field{},

	}

	typeHierarchy := getTypeHierarchy(name, T, types)
	ramlTypes := make([]raml.Type, 0)
	for _, v := range typeHierarchy {
		for _, iv := range v {
			ramlTypes = append(ramlTypes, iv)
		}
	}
	properties := getTypeProperties(ramlTypes)

	for k, v := range properties {
		op := objectProperties(k, v)
		field, err := newField(name, T, raml.ToProperty(k, v), types, op, typeHierarchy)
		if err != nil {
			continue
		}
	
		pc.Fields[field.Name] = field
	}
	return pc
}


func objectProperties(name string, p interface{}) []objectProperty {
	props := make([]objectProperty, 0)

	switch prop := p.(type) {
	case map[interface{}]interface{}:
		ramlProp := raml.ToProperty(name, p)
		if ramlProp.Type == "object" {
			for k, v := range prop {
				switch k {
				case "properties":
					for propName, childProp := range v.(map[interface{}]interface{}) {
						rProp := raml.ToProperty(propName.(string), childProp)
						objprop := objectProperty {
							name: rProp.Name,
							required: rProp.Required,
							datatype: rProp.Type,
						}
						if rProp.Type == "object" {
							objprop.childProperties = append(objprop.childProperties, objectProperties(propName.(string), childProp)...)	
						}
						props = append(props, objprop)
					}		
				}
			}
		}
	}

	return props
}


// func childProperties(p interface{}) []raml.Property {
// 	props := make([]raml.Property, 0)

// 	switch prop := p.(type) {
// 	case map[interface{}]interface{}:
// 		for k, v := range prop {
// 			if k == "properties" {
// 				for propName, childProp := range v.(map[interface{}]interface{}) {
// 					props = append(raml.ToProperty(propName.(string), childProp), property)
// 				}
// 			}
// 		}
// 	}

// 	return props
// }


func getTypeHierarchy(name string, T raml.Type, types map[string]raml.Type) []map[string]raml.Type {
	typelist := []map[string]raml.Type{map[string]raml.Type{name: T}}

	parentType, inherited := types[T.Type.(string)]
	if inherited {
		for _, pt := range getTypeHierarchy(T.Type.(string), parentType, types) {
			typelist = append(typelist, pt)
		}
	}

	return typelist
}


func getTypeProperties(typelist []raml.Type) map[string]interface{} {
	// get a list of the types in the inheritance chain for T
	// walk it from the top down and add the properties
	properties := make(map[string]interface{})
	for i := len(typelist)-1; i >= 0; i-- {
		for k, v := range typelist[i].Properties {
			properties[k] = v
		}
	}

	return properties
}


func newClassFromType(T raml.Type, name string, types map[string]raml.Type) class {
	pc := newClass(name, T, types)
	// pc.T = T
	pc.handleAdvancedType()
	return pc
}

// generate a python class file
func (pc *class) generate(dir string) error {
	// generate enums
	for _, f := range pc.Fields {
		if f.Enum != nil {
			// TODO if this enum is inherited from a parent class without changes, skip
			// if pc.Name == "ClientError" {
			// 	fmt.Println("calling enum generate for", pc.Name, f.Name)
			// 	fmt.Printf("\n%+v\n", pc)
			// }
			if err := f.Enum.generate(dir); err != nil {
				return err
			}
		}
	}

	if pc.Enum != nil {
		return pc.Enum.generate(dir)
	}

	fileName := filepath.Join(dir, pc.Name+".py")
	return commons.GenerateFile(pc, "./templates/class_python.tmpl", "class_python", fileName, false)
}

func (pc *class) handleAdvancedType() {
	if pc.T.Type == nil {
		pc.T.Type = "object"
	}
	if pc.T.IsEnum() {
		pc.Enum = newEnumFromClass(pc)
	}
}

// generate all classes from all  methods request/response bodies
// func generateClassesFromBodies(rs []pythonResource, dir string) error {
// 	for _, r := range rs {
// 		for _, mi := range r.Methods {
// 			m := mi.(serverMethod)
// 			if err := generateClassesFromMethod(m, dir); err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// generate classes from a method
//
// TODO:
// we currently camel case instead of snake case because of mistake in previous code
// and we might need to maintain backward compatibility. Fix this!
// func generateClassesFromMethod(m serverMethod, dir string) error {
// 	// request body
// 	if commons.HasJSONBody(&m.Bodies) {
// 		name := inflect.UpperCamelCase(m.MethodName + "ReqBody")
// 		class := newClass(name, "", m.Bodies.ApplicationJSON.Properties)
// 		if err := class.generate(dir); err != nil {
// 			return err
// 		}
// 	}

// 	// response body
// 	for _, r := range m.Responses {
// 		if !commons.HasJSONBody(&r.Bodies) {
// 			continue
// 		}
// 		name := inflect.UpperCamelCase(m.MethodName + "RespBody")
// 		class := newClass(name, "", r.Bodies.ApplicationJSON.Properties)
// 		if err := class.generate(dir); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }


// return list of import statements
func (pc class) Imports() []string {
	// var imports []string
	imports := make(map[string]bool)

	for _, field := range pc.Fields {
		for _, imp := range field.imports {
			importString := "from " + imp.Module + " import " + imp.Name
			imports[importString] = true
		}
	}
	var importStrings []string
	for key := range imports {
		importStrings = append(importStrings, key)
	}
	sort.Strings(importStrings)
	return importStrings
}

// generate all python classes from an RAML document
func generateClasses(types map[string]raml.Type, dir string) error {
	for k, t := range types {
		pc := newClassFromType(t, k, types)
		if err := pc.generate(dir); err != nil {
			return err
		}
	}
	return nil
}
