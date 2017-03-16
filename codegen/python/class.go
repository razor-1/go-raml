package python

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chuckpreslar/inflect"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/raml"
)

// class defines a python class
type class struct {
	T                 raml.Type
	Name              string
	Description       []string
	Fields            map[string]field
	Enum              *enum
	CreateParamString string
}

type objectProperty struct {
	name            string
	required        bool
	datatype        string
	childProperties []objectProperty
}

// create a python class representations
func newClass(name string, description string, properties map[string]interface{}) class {
	pc := class{
		Name:        name,
		Description: commons.ParseDescription(description),
		Fields:      map[string]field{},
	}

	types := globAPIDef.Types
	T := types[name]

	typeHierarchy := getTypeHierarchy(name, T, types)
	ramlTypes := make([]raml.Type, 0)
	for _, v := range typeHierarchy {
		for _, iv := range v {
			ramlTypes = append(ramlTypes, iv)
		}
	}
	properties = getTypeProperties(ramlTypes)

	for propName, propInterface := range properties {
		op := objectProperties(propName, propInterface)
		// field, err := newField(name, T, raml.ToProperty(k, v), types, op, typeHierarchy)
		field, err := newField(name, T, propName, propInterface, types, op, typeHierarchy)
		if err != nil {
			continue
		}

		pc.Fields[field.Name] = field
	}

	// build the CreateParamString, used as part of the create() staticmethod
	// which is a convenience initializer for the class
	requiredFields := make([]string, 0)
	optionalFields := make([]string, 0)
	for fieldName, fieldVal := range pc.Fields {
		if fieldVal.Required {
			requiredFields = append(requiredFields, fieldName)
		} else {
			optionalFields = append(optionalFields, fmt.Sprintf("%s=None", fieldName))
		}
	}
	// sort them so we have some stability in param order (important for requiredFields)
	sort.Strings(requiredFields)
	sort.Strings(optionalFields)
	requiredString := strings.Join(requiredFields, ", ")
	optionalString := strings.Join(optionalFields, ", ")

	if len(requiredFields) > 0 && len(optionalFields) > 0 {
		combinedString := []string{requiredString, optionalString}
		pc.CreateParamString = strings.Join(combinedString, ", ")
	} else {
		if len(requiredFields) > 0 {
			pc.CreateParamString = requiredString
		} else if len(optionalFields) > 0 {
			pc.CreateParamString = optionalString
		}
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
						objprop := objectProperty{
							name:     rProp.Name,
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

func ChildProperties(Properties map[string]interface{}) []raml.Property {
	props := make([]raml.Property, 0)

	for propName, propInterface := range Properties {
		props = append(props, raml.ToProperty(propName, propInterface))
	}

	return props
}

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
	for i := len(typelist) - 1; i >= 0; i-- {
		for k, v := range typelist[i].Properties {
			properties[k] = v
		}
	}

	return properties
}

func newClassFromType(T raml.Type, name string) class {
	pc := newClass(name, T.Description, T.Properties)
	pc.T = T
	pc.handleAdvancedType()
	return pc
}

// generate a python class file
func (pc *class) generate(dir string) (error, []string) {
	// generate enums
	typeNames := make([]string, 0)
	for _, f := range pc.Fields {
		if f.Enum != nil {
			typeNames = append(typeNames, f.Enum.Name)
			if err := f.Enum.generate(dir); err != nil {
				return err, typeNames
			}
		}
	}

	if pc.Enum != nil {
		typeNames = append(typeNames, pc.Enum.Name)
		return pc.Enum.generate(dir), typeNames
	}

	fileName := filepath.Join(dir, pc.Name+".py")
	typeNames = append(typeNames, pc.Name)
	return commons.GenerateFile(pc, "./templates/class_python.tmpl", "class_python", fileName, false), typeNames
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
func (fs FlaskServer) generateClassesFromBodies(dir string) error {
	for _, r := range fs.ResourcesDef {
		for _, mi := range r.Methods {
			m := mi.(serverMethod)
			if err := generateClassesFromMethod(m, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// generate classes from a method
//
// TODO:
// we currently camel case instead of snake case because of mistake in previous code
// and we might need to maintain backward compatibility. Fix this!
func generateClassesFromMethod(m serverMethod, dir string) error {
	// request body
	if commons.HasJSONBody(&m.Bodies) {
		name := inflect.UpperCamelCase(m.MethodName + "ReqBody")
		class := newClass(name, "", m.Bodies.ApplicationJSON.Properties)
		if err, _ := class.generate(dir); err != nil {
			return err
		}
	}
	return nil
}

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
func generateClasses(types map[string]raml.Type, dir string) (error, []string) {
	typeNames := make([]string, 0)
	for k, t := range types {
		// this is special; ignore it, Python has a native module for this
		if k == "UUID" {
			continue
		}
		pc := newClassFromType(t, k)
		err, types := pc.generate(dir)
		typeNames = append(typeNames, types...)
		if err != nil {
			return err, typeNames
		}
	}
	return nil, typeNames
}
