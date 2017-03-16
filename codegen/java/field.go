package java

import (
	"fmt"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/raml"
)

// java class's field
type field struct {
	Name        string
	CamelCaseName string
	Type        string
	Required    bool 				// if the field itself is required
	DataType    string 				// the python datatype (objmap) used in the template
	HasChildProperties bool
	RequiredChildProperties []string
	Validators  string
	Enum        *enum
	isFormField bool
	imports     []string
	UnionTypes  []string
	IsList      bool                // it is a list field
	validators  map[string][]string // array of validators, only used to build `Validators` field
	CustomType  bool
	IsMember    bool 				// if this property should be a member of this class. 
	Annotations []string 			// annotations for this field, e.g. @JsonFormat(shape = JsonFormat.Shape.STRING) for Jackson
}

var javaClass class

func newField(jc class, T raml.Type, prop raml.Property, childProperties []objectProperty, typeHierarchy []map[string]raml.Type, isMember bool) (field, error) {
	types := globAPIDef.Types
	className := jc.Name
	javaClass = jc

	f := field {
		Name:     prop.Name,
		CamelCaseName: strings.Title(prop.Name),
		Required: prop.Required,
		IsMember: isMember,
	}

	if prop.IsEnum() {
		if f.Enum == nil {
			f.Enum = newEnum(className, prop, true)
		}
		// need Map and HashMap for Enums
		f.imports = []string {"java.util.Map", "java.util.HashMap"}
		switch jc.Annotations {
		case annotationJackson:
			f.imports = append(f.imports,
				"com.fasterxml.jackson.annotation.JsonCreator",
				"com.fasterxml.jackson.annotation.JsonValue")
		case annotationGSON:
			f.Enum.AnnotationGSON = true
		}
		f.Type = className + "." + f.CamelCaseName
	} else {
		f.setType(prop.Type)
		if f.Type == "" {
			return f, fmt.Errorf("unsupported type:%v", prop.Type)
		}
	}

	if jc.GSON {
		f.Annotations = []string {"@SerializedName(\""+f.Name+"\")", "@Expose"}
	}

	// see if there are different required properties for this instance of a type vs. the type's main declaration
    mainRequired := make([]string, 0)
    childRequired := make([]string, 0)
    if mainType, ok := types[f.Type]; ok {
    	for mainName, mainInterface := range mainType.Properties {
    		mainProp := raml.ToProperty(mainName, mainInterface)
    		if mainProp.Required {
    			mainRequired = append(mainRequired, strings.Title(mainProp.Name))
    		}
    	}
    	for _, typeProp := range prop.Properties {
    		if typeProp.Required {
    			childRequired = append(childRequired, strings.Title(typeProp.Name))
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

    if _, ok := types[prop.Type]; ok {
    	f.CustomType = true
    }
    if len(f.UnionTypes) > 0 {
    	f.CustomType = true
    }

	return f, nil
}


// convert from raml Type to java type
func (jf *field) setType(t string) {
	// base RAML types we can directly map:
	switch t {
	case "string":
		jf.Type = "String"
	case "integer", "number":
		// not dealing with floats here
		jf.Type = "Integer"
	case "boolean":
		jf.Type = "Boolean"
	case "datetime":
		jf.Type = "Instant"
		jf.imports = []string {
			"java.time.Instant",
		}
		switch javaClass.Annotations {
		case annotationJackson:
			jf.Annotations = []string {"@JsonFormat(shape = JsonFormat.Shape.STRING)"}
			jf.imports = append(jf.imports, "com.fasterxml.jackson.annotation.JsonFormat")
		}
	case "object":
		jf.Type = "Map<String, Object>"
		jf.imports = append(jf.imports, "java.util.Map")
	}

	// special types we want to hard code
	switch t {
		case "UUID":
		jf.Type = t
		jf.imports = []string {
			"java.util.UUID",
		}
	}

	if jf.Type != "" { // type already set, no need to go down
		return
	}

	// other types that need some processing
	switch {
	case strings.HasSuffix(t, "[][]"): // bidimensional array
		log.Info("validator has no support for bidimensional array, ignore it")
	case strings.HasSuffix(t, "[]"): // array
		jf.IsList = true
		jf.setType(fmt.Sprintf("List<%s>", t[:len(t)-2]))
		jf.imports = append(jf.imports, "java.util.List")
	case strings.HasSuffix(t, "{}"): // map
		log.Info("validator has no support for map, ignore it")
	case strings.Index(t, "|") > 0:
		// send the list of union types to the template
		for _, ut := range strings.Split(t, "|") {
			typename := strings.TrimSpace(ut)
			jf.UnionTypes = append(jf.UnionTypes, typename)
		}
		jf.Type = GetCommonParentType(jf.UnionTypes)
	case strings.Index(t, ".") > 1:
		jf.Type = t[strings.Index(t, ".")+1:]
	default:
		jf.Type = t
	}

}

/*
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
*/

