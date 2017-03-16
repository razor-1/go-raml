package java

import (
	"path/filepath"
	"sort"
	// "fmt"
	"regexp"
	"strings"
	"math"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/raml"
)

// class defines a java class
type class struct {
	T           raml.Type
	Name        string
	Description []string
	Fields      map[string]field
	Enum        *enum
	PackageName string
	HasParentType bool
	ParentType string
	HasChildType bool      // if this type has any children that inherit from it
	AbstractType bool 	   // is this type used as the type of a property/method anywhere? or just an empty 'parent' type that is only extended?
	HasRequiredProperties bool // true if there is at least one requred property
	Annotations string     // if not an empty string, either "jackson" or "gson" specifies the annotation style
	Jackson  	bool 	   // if we're doing jackson annotations, set to true
	GSON 		bool  	   // if we're doing GSON annotations, set to true
	Deserializer string    // if we need a custom deserializer for this type, this will be the name of the class to use
}

type objectProperty struct {
	name 			string
	required 		bool
	datatype 		string
	childProperties []objectProperty
}

var javaClient Client
var outputDir  string

// create a java class representation
func newClass(name string, T raml.Type) class {
	jc := class{
		Name:        name,
		Description: commons.ParseDescription(T.Description),
		Fields:      map[string]field{},
		PackageName: javaClient.PackageName,
		Annotations: javaClient.Annotations,
	}

	switch jc.Annotations {
	case annotationJackson:
		jc.Jackson = true
	case annotationGSON:
		jc.GSON = true
	}

	typeHierarchy := getTypeHierarchy(name, T)

	if parentTypeName, ok := getParentType(name); ok {
		jc.HasParentType = true
		jc.ParentType = parentTypeName
	}

	// go through every type, and see if this type is the parent. if so, set HasChildType
	childTypes := make([]raml.Type, 0)
	childTypeNames := make([]string, 0)
	for typeName, ramlType := range globAPIDef.Types {
		if parentTypeName, ok := getParentType(typeName); ok && parentTypeName == name {
			jc.HasChildType = true
			childTypes = append(childTypes, ramlType)
			childTypeNames = append(childTypeNames, typeName)
		}
	}

	if !propertyUsesType(name) && !resourceUsesType(name) && !jc.HasParentType {
		jc.AbstractType = true
		if needsCustomDeserializer(name) {
			jc.Deserializer = customDeserializer(jc, childTypeNames)
		}
	}

	if jc.HasParentType && needsCustomDeserializer(jc.ParentType) {
		// my parent type has a custom deserializer. i need to set my deserializer to myself
		jc.Deserializer = name
	}


	ramlTypes := []raml.Type {T}
	properties := getTypeProperties(ramlTypes)

	for propName, propInterface := range T.Properties {
		ramlProp := raml.ToProperty(propName, propInterface)
		if ramlProp.Required {
			jc.HasRequiredProperties = true
			break
		}
	}

	for propName, v := range properties {
		isMember := true
		ramlProp := raml.ToProperty(propName, v)
		// exclude properties that are defined in abstract parents and in all child types
		if jc.AbstractType {
			missing := false
			for _, childType := range childTypes {
				if _, ok := childType.Properties[propName]; !ok {
					missing = true
				}
			}
			if !missing {
				continue
			}
		} else if jc.HasParentType {
			// if my parent type has this property, and it uses the same Type there, set isMember to false
			parentType := globAPIDef.Types[jc.ParentType]
			if parentTypeProperty, ok := parentType.Properties[propName]; ok {
				ptRamlProp := raml.ToProperty(propName, parentTypeProperty)
				if ptRamlProp.Type == ramlProp.Type && !ramlProp.IsEnum() {
					isMember = false
				}
			}
		}
		op := objectProperties(propName, v)
		field, err := newField(jc, T, ramlProp, op, typeHierarchy, isMember)
		if err != nil {
			continue
		}
	
		jc.Fields[field.Name] = field
	}
	return jc
}


func needsCustomDeserializer(forTypeName string) bool {
	// determine if the type forTypeName would need a custom deserializer
	// e.g. EventType needs one so that ClientEvent or ServiceEvent can be selected
	// deserialization happens based on the 'name' property (see the template)

	// have to have an annotation style (Jackson/GSON) for this to make sense
	if javaClient.Annotations == "" {
		return false
	}

	// if a property or resources uses this type, then it's not an abstract parent that needs a custom deserializer
	if propertyUsesType(forTypeName) || resourceUsesType(forTypeName) {
		return false
	}

	// if this type has a parent type, then it's not an abstract parent either
	if _, ok := getParentType(forTypeName); ok {
		return false
	}

	// get all the child types of forTypeName
	childTypeNames := make([]string, 0)
	for typeName, _ := range globAPIDef.Types {
		if parentTypeName, ok := getParentType(typeName); ok && parentTypeName == forTypeName {
			childTypeNames = append(childTypeNames, typeName)
		}
	}

	// get all union properties in the entire spec and see if any of them have ALL the child types
	for _, unionProperty := range getUnionProperties() {
		missing := false
		unionPropertyTypeNames := normalizedTypeNames(unionProperty.Type)
		for _, childTypeName := range childTypeNames {
			if slicePosition(childTypeName, unionPropertyTypeNames) == -1 {
				missing = true
				break
			}
		}
		if !missing {
			return true
		}
	}

	return false
}


func customDeserializer(jc class, childTypeNames []string) string {
	// generate custom deserializer file
	
	deserializerName := jc.Name + "Deserializer"
	fileName := filepath.Join(outputDir, deserializerName+".java")
	deserializerData := struct {
		Name 			string
		PackageName 	string
		ChildClasses	[]string
	} {
		jc.Name,
		jc.PackageName,
		childTypeNames,
	}
	// select the jackson or gson template
	commons.GenerateFile(deserializerData, "./templates/Deserializer."+jc.Annotations+".java.tmpl",
		"deserializer_"+jc.Annotations+"_java", fileName, false)

	return deserializerName
}


func slicePosition(needle string, haystack []string) int {
	for pos, check := range haystack {
		if check == needle { return pos }
	}

	return -1
}


func getUnionProperties() []raml.Property {
	// get all raml.Property from all types where IsUnion is true
	unionProps := make([]raml.Property, 0)
	for _, ramlType := range globAPIDef.Types {
		for propName, propInterface := range ramlType.Properties {
			unionProps = append(unionProps, getUnions(raml.ToProperty(propName, propInterface))...)
		}
	}

	return unionProps
}


func getUnions(ramlProp raml.Property) []raml.Property {
	// get all raml.Property where IsUnion is true
	unionProps := make([]raml.Property, 0)
	if ramlProp.IsUnion() {
		unionProps = append(unionProps, ramlProp)
	}
	for _, subProperty := range ramlProp.Properties {
		unionProps = append(unionProps, getUnions(subProperty)...)
	}

	return unionProps
}


func GetCommonParentType(types []string) string {
	// find the closest common parent type to the elements in types
	// default to 'Object'
	commonParent := "Object"
	ramlTypes := globAPIDef.Types
	hierarchies := make(map[string][]string)

	// first, get all the parent types
	for _, t := range types {
		typeHierarchy := make([]string, 0)
		if ramlType, ok := ramlTypes[t]; ok {
			for idx, hierMap := range getTypeHierarchy(t, ramlType) {
				if idx == 0 { continue }
				for typeName, _ := range hierMap {
					typeHierarchy = append(typeHierarchy, typeName)
				}
			}
		}
		hierarchies[t] = typeHierarchy
	}

	// now, go through each and look for commonalities
	var bestIdx int = math.MaxInt32
	for _, parentNames := range hierarchies {
		for _, parentName := range parentNames {
			foundIdx := make(map[string]int)
			for typeName2, parentNames2 := range hierarchies {
				pos := slicePosition(parentName, parentNames2)
				if pos >= 0 {
					foundIdx[typeName2] = pos
				} else {
					break
				}
			}
			if len(foundIdx) == len(hierarchies) {
				// get the largest value in foundIdx
				foundInts := make([]int, 0)
				for _, foundInt := range foundIdx {
					foundInts = append(foundInts, foundInt)
				}
				sort.Sort(sort.Reverse(sort.IntSlice(foundInts)))
				if foundInts[0] < bestIdx {
					bestIdx = foundInts[0]
					commonParent = parentName
				}
			}
		}
	}

	// fmt.Println("hierarchies", hierarchies)
	// fmt.Println("common parent", commonParent)

	return commonParent
}


func getParentType(typeName string) (string, bool) {
	typeHierarchy := getTypeHierarchy(typeName, globAPIDef.Types[typeName])
	if len(typeHierarchy) < 2 {
		return "", false
	}

	for parentTypeName, _ := range typeHierarchy[1] {
		return parentTypeName, true
	}

	return "", false
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



func getDescendentPropertyTypes(prop raml.Property) map[string]bool {
	propTypes := make(map[string]bool)

	for _, tname := range normalizedTypeNames(prop.Type) {
		propTypes[tname] = true
	}

	for _, childProp := range prop.Properties {
		for cType, _ := range getDescendentPropertyTypes(childProp) {
			propTypes[cType] = true
		}
	}

	return propTypes
}


func normalizedTypeNames(typeName string) []string {
	typeSet := make(map[string]bool)

	if strings.Index(typeName, "|") > 0 {
		// add all union types
		r := regexp.MustCompile(`\s*\|\s*`)
		for _, unionType := range r.Split(typeName, -1) {
			typeSet[unionType] = true
		}
	} else {
		if strings.HasSuffix(typeName, "[]") {
			// strip off the array suffix and add the type name
			r := regexp.MustCompile(`\s*\[\]\s*`)
			typeName = r.ReplaceAllString(typeName, "")
		}
		typeSet[typeName] = true
	}

	typeNames := make([]string, 0)
	for tname, _ := range typeSet {
		typeNames = append(typeNames, tname)
	}

	return typeNames
}

func resourceUsesType(needle string) bool {
	// see if needle is the type of any method body on any resource

	for _, resource := range globAPIDef.Resources {
		for _, method := range resource.Methods {
			for _, typeName := range normalizedTypeNames(method.Bodies.Type) {
				if typeName == needle { return true }
			}
		}
	}

	return false
}

func propertyUsesType(needle string) bool {
	types := globAPIDef.Types
	// see if needle is the type of any property anywhere in types
	allPropertyTypes := make(map[string]bool)

	for _, ramlType := range types {
		for propName, propInterface := range ramlType.Properties {
			for cType, _ := range getDescendentPropertyTypes(raml.ToProperty(propName, propInterface)) {
				allPropertyTypes[cType] = true
			}
		}

	}

	_, found := allPropertyTypes[needle]

	return found
}


func getTypeHierarchy(name string, T raml.Type) []map[string]raml.Type {
	typelist := []map[string]raml.Type{map[string]raml.Type{name: T}}

	parentType, inherited := globAPIDef.Types[T.Type.(string)]
	if inherited {
		for _, pt := range getTypeHierarchy(T.Type.(string), parentType) {
			typelist = append(typelist, pt)
		}
	}

	return typelist
}


func getTypeProperties(typelist []raml.Type) map[string]interface{} {
	// get all the properties of types contained in typelist
	properties := make(map[string]interface{})
	for i := len(typelist)-1; i >= 0; i-- {
		for k, v := range typelist[i].Properties {
			properties[k] = v
		}
	}

	return properties
}


func newClassFromType(T raml.Type, name string) class {
	jc := newClass(name, T)
	jc.handleAdvancedType()
	return jc
}

// generate a java class file
func (jc *class) generate() error {
	// generate enums
	for _, f := range jc.Fields {
		if f.Enum != nil {
			if err := f.Enum.generate(outputDir); err != nil {
				return err
			}
		}
	}

	if jc.Enum != nil {
		return jc.Enum.generate(outputDir)
	}

	fileName := filepath.Join(outputDir, jc.Name+".java")
	return commons.GenerateFile(jc, "./templates/class_java.tmpl", "class_java", fileName, false)
}

func (jc *class) handleAdvancedType() {
	if jc.T.Type == nil {
		jc.T.Type = "object"
	}
	if jc.T.IsEnum() {
		jc.Enum = newEnumFromClass(jc)
	}
}


// return list of import statements
func (jc class) Imports() []string {
	imports := make(map[string]bool)

	for _, field := range jc.Fields {
		for _, imp := range field.imports {
			imports[imp] = true
		}
	}

	switch jc.Annotations {
	case annotationJackson:
		imports["com.fasterxml.jackson.annotation.JsonInclude"] = true
		if jc.Deserializer != "" {
			imports["com.fasterxml.jackson.databind.annotation.JsonDeserialize"] = true
		}
	case annotationGSON:
		imports["com.google.gson.annotations.Expose"] = true
		imports["com.google.gson.annotations.SerializedName"] = true
	}

	var importStrings []string
	for key := range imports {
		importStrings = append(importStrings, key)
	}
	sort.Strings(importStrings)
	return importStrings
}

// generate all java classes from a RAML document
func generateClasses(dir string, c Client) error {
	outputDir = dir
	javaClient = c
	for typeName, ramlType := range globAPIDef.Types {
		// skip UUID; Java has a native type for it
		if typeName == "UUID" {
			continue
		}
		jc := newClassFromType(ramlType, typeName)
		if err := jc.generate(); err != nil {
			return err
		}
	}
	return nil
}
