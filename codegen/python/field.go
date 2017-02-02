package python

import (
	"fmt"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/raml"
)

// import package and name
type pyimport struct {
	Module string
	Name   string
}

// pythons class's field
type field struct {
	Name        string
	Type        string
	Required    bool 				// if the field itself is required
	DataType    string 				// the python datatype (objmap) used in the template
	HasChildProperties bool
	RequiredChildProperties []string
	Validators  string
	Enum        *enum
	isFormField bool
	imports     []pyimport
	UnionTypes  []string
	// Initializer string
	IsList      bool                // it is a list field
	validators  map[string][]string // array of validators, only used to build `Validators` field
}

func newField(className string, T raml.Type, propName string, propInterface interface{},
	types map[string]raml.Type, childProperties []objectProperty,
	typeHierarchy []map[string]raml.Type) (field, error) {
	
	// if className == "ClientEvent" {
	// 	// debug TODO remove
	// 	fmt.Println(className, propName, propInterface)
	// 	fmt.Println(childProperties)
	// 	fmt.Println("----")
	// }

	prop := raml.ToProperty(propName, propInterface)

	f := field {
		Name:     prop.Name,
		Required: prop.Required,
	}

	// for _, childProp := range childProperties {
	// 	f.RequiredProperties = append(f.RequiredProperties, childProp.Name)
	// }

	if prop.IsEnum() {
		// see if T actually has this property. if not, it's inherited, and we want to only define an Enum once
		// first, get the name of the type that actually defines this property, in the inheritance chain:
		typeDefiningProp := func() string {
			for _, typeMap := range typeHierarchy {
				for typeName, typeVal := range typeMap {
					for k, v := range typeVal.Properties {
						tempProp := raml.ToProperty(k, v)
						if tempProp.Name == prop.Name {
							return typeName
						}
					}
				}
			}
			return ""
		}()

		if typeDefiningProp != className {
			// this enum property isn't actually defined on T. it's inherited from a parent type
			// thus, we'll use the parent type's enum

			f.Enum = newEnum(typeDefiningProp, prop, false)
			// fmt.Println("*** typeDefining skip enum", typeDefiningProp, className, prop.Name)
			// fmt.Printf("full type: %+v\n", T)
		}
		if f.Enum == nil {
			f.Enum = newEnum(className, prop, false)
		}
		f.Type = f.Enum.Name
		f.imports = []pyimport {
			pyimport {
				Module: f.Type,
				Name: f.Type,
			},
		}
	} else {
		f.setType(prop.Type)
		if f.Type == "" {
			return f, fmt.Errorf("unsupported type:%v", prop.Type)
		}
		f.buildValidators(prop)
	}

	f.DataType, f.HasChildProperties = buildDataType(f, childProperties)

	// see if there are different required properties for this instance of a type vs. the type's main declaration
	mainRequired := make([]string, 0)
	childRequired := make([]string, 0)
	if mainType, ok := types[f.Type]; ok {
		switch thisProp := propInterface.(type) {
			case map[interface{}]interface{}:
				if myChildProperties, ok := thisProp["properties"].(map[interface{}]interface{}); ok {
					for _, typeProp := range ChildProperties(mainType.Properties) {
						if typeProp.Required {
							mainRequired = append(mainRequired, typeProp.Name)
						}
					}

					myChildPropertyMap := make(map[string]interface{})
					for k, v := range myChildProperties {
						if childPropName, ok := k.(string); ok {
							myChildPropertyMap[childPropName] = v
						}
					}

					for _, myProp := range ChildProperties(myChildPropertyMap) {
						if myProp.Required {
							childRequired = append(childRequired, myProp.Name)
						}
					}
				}
		}
	}
	if len(childRequired) > len(mainRequired) {
		// some properties were made required and we need to validate them
		// sort the lists so we can get only the fields that are required on this child
		sort.Strings(childRequired)
		sort.Strings(mainRequired)

		f.RequiredChildProperties = childRequired[len(mainRequired):]
	}

	return f, nil
}


func buildDataType(f field, childProperties []objectProperty) (string, bool) {
	/*
	build a string for the 'datatype' key of an objmap for this property
	a complete objmap looks like:
	{'attrname': {'datatype': [type], 'required': bool}}

	the type values in the 'datatype' list can be any type, but if they are a dict, it's in objmap format

	there can be many levels of nesting, but here is an example of one:
	{
		'sipSessionId': {
			'datatype': [
				{
					'local': {'datatype': [str], 'required': False},
					'remote': {'datatype': [str], 'required': False},
				},
			],
			'required': False
		}
	}
	*/

	if len(f.UnionTypes) > 0 {
		return strings.Join(f.UnionTypes, ", "), false
	}
	if f.Type != "dict" || len(childProperties) == 0 {
		return f.Type, false
	}

	// we have a dict with child properties of type 'object'. build the datatype string
	// fmt.Println("childprops for", f.Type, childProperties)
	var datatypes []string
	for _, objProp := range childProperties {
		// fmt.Println("have childprop", objProp)
		reqstr := "True"
		if !objProp.required {
			reqstr = "False"
		}
		childField := field {
			Name: objProp.name,
		}
		childField.setType(objProp.datatype)
		thisDatatype := childField.Type
		if len(objProp.childProperties) > 0 {
			thisDatatype, _ = buildDataType(childField, objProp.childProperties)
		}
		thisProp := fmt.Sprintf("'%s': {'datatype': [%s], 'required': %s}", objProp.name, thisDatatype, reqstr)
		datatypes = append(datatypes, thisProp)
	}

	return strings.Join(datatypes, ", "), true
}


// convert from raml Type to python type
func (pf *field) setType(t string) {
	// base RAML types we can directly map:
	switch t {
	case "string":
		pf.Type = "str"
	case "integer", "number":
		// not dealing with floats here
		pf.Type = "int"
	case "boolean":
		pf.Type = "bool"
	case "datetime":
		pf.Type = t
		pf.imports = []pyimport {
			pyimport {
				Module: "datetime",
				Name: "datetime",
			},
		}
		// pf.Initializer = "timestamp_to_datetime"
	case "object":
		pf.Type = "dict"
	}

	// special types we want to hard code
	switch t {
		case "UUID":
		pf.Type = t
		pf.imports = []pyimport {
			pyimport {
				Module: "uuid",
				Name: "UUID",
			},
		}
	}

	if pf.Type != "" { // type already set, no need to go down
		return
	}

	// rt, found := pf.ramlType.(raml.Type)
	// if !found {
	// 	return
	// }

	// other types that need some processing
	switch {
	case strings.HasSuffix(t, "[][]"): // bidimensional array
		log.Info("validator has no support for bidimensional array, ignore it")
	case strings.HasSuffix(t, "[]"): // array
		pf.IsList = true
		pf.setType(t[:len(t)-2])
	case strings.HasSuffix(t, "{}"): // map
		log.Info("validator has no support for map, ignore it")
	case strings.Index(t, "|") > 0:
		// send the list of union types to the template
		for _, ut := range strings.Split(t, "|") {
			typename := strings.TrimSpace(ut)
			pf.UnionTypes = append(pf.UnionTypes, typename)
			pf.imports = append(pf.imports, pyimport {
				Module: typename,
				Name: typename,
				})
			pf.Type = t
		}
	case strings.Index(t, ".") > 1:
		pf.Type = t[strings.Index(t, ".")+1:]
	default:
		pf.Type = t
		pf.imports = []pyimport {
			pyimport {
				Module: t,
				Name: t,
			},
		}
	}

}

func (pf *field) addValidator(name, arg string, val interface{}) {
	pf.validators[name] = append(pf.validators[name], fmt.Sprintf("%v=%v", arg, val))
}

// build validators string
func (pf *field) buildValidators(p raml.Property) {
	pf.validators = map[string][]string{}
	// string
	if p.MinLength != nil {
		pf.addValidator("Length", "min", *p.MinLength)
	}
	if p.MaxLength != nil {
		pf.addValidator("Length", "max", *p.MaxLength)
	}
	if p.Pattern != nil {
		pf.addValidator("Regexp", "regex", `"`+*p.Pattern+`"`)
	}

	// number
	if p.Minimum != nil {
		pf.addValidator("NumberRange", "min", *p.Minimum)
	}
	if p.Maximum != nil {
		pf.addValidator("NumberRange", "max", *p.Maximum)
	}
	if p.MultipleOf != nil {
		pf.addValidator("multiple_of", "mult", *p.MultipleOf)
	}

	// required
	if p.Required {
		pf.addValidator("DataRequired", "message", `""`)
	}

	if p.MinItems != nil {
		pf.Validators += fmt.Sprintf(",min_entries=%v", *p.MinItems)
	}
	if p.MaxItems != nil {
		pf.Validators += fmt.Sprintf(",max_entries=%v", *p.MaxItems)
	}
	if len(pf.Validators) > 0 {
		pf.Validators = pf.Validators[1:]
	}

	pf.buildValidatorsString()
}

func (pf *field) buildValidatorsString() {
	var v []string
	if pf.Validators != "" {
		return
	}
	for name, args := range pf.validators {
		v = append(v, fmt.Sprintf("%v(%v)", name, strings.Join(args, ", ")))
	}

	// we actually don't need to sort it to generate correct validators
	// we need to sort it to generate predictable order which needed during the test
	sort.Strings(v)
	pf.Validators = strings.Join(v, ", ")
}

// WTFType return wtforms type of a field
func (pf field) WTFType() string {
	switch {
	case pf.IsList && pf.isFormField:
		return fmt.Sprintf("FieldList(FormField(%v))", pf.Type)
	case pf.IsList:
		return fmt.Sprintf("FieldList(%v('%v', [required()]), %v)", pf.Type, pf.Name, pf.Validators)
	case pf.isFormField:
		return fmt.Sprintf("FormField(%v)", pf.Type)
	default:
		return fmt.Sprintf("%v(validators=[%v])", pf.Type, pf.Validators)
	}
}
