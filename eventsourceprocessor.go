package eventsourceprocessor

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

/*
	Exported Structs, Interfaces, Functions, Consts and configuration.
*/

// Document is a snapshot + any new events which have not been applied to that snapshot.
type Document struct {
	EntityId     string          `json:"EntityId"`     // The ID of the document we're getting
	BaseDocument []byte          `json:"BaseDocument"` // The base document.
	Events       []DocumentEvent `json:"Events"`       // Array of events, in the order they were posted, to apply to the base document
}

// DocumentEvent is a single "business" event to apply to a document. It may consist of many instructions,
// due to the way the data is processed.
type DocumentEvent struct {
	EventId      uuid.UUID          // Unique ID for this event.
	Timestamp    uint64             // Unix style timestamp, in microseconds.
	Instructions []EventInstruction `json:"Instructions"` // Collection of instructions on how to apply this event to the document.
}

// EventInstruction describes a thing to do to the master document.
// See the README.md file in this directory for a how-it-works guide.
type EventInstruction struct {
	Path       string     // Path to the element in the root document.
	ActionType ActionType // Action to take at the path supplied (e.g. addOrUpdate, append, delete)
	DataType   DataType   // e.g. "string","float64","bool", "map", "array" or "null"
	Value      string     // Value, must be valid for the datatype. Ignored for "null"
}

// Action types
type ActionType string

const (
	ActionTypeSetOrAdd ActionType = "SetOrAdd" // Add value, or set (overwrite) it if value is already present
	ActionTypeAddOnly  ActionType = "AddOnly"  // Add the value. Do NOT overwrite it if the value is already present
	ActionTypeSetOnly  ActionType = "SetOnly"  // Update a value. Do NOT add it, if it's not already present
	ActionTypeRemove   ActionType = "Remove"   // Remove a value. Obviously, do nothing if it's not present.
)

// Data types
type DataType string

const (
	DataTypeNone   DataType = ""
	DataTypeString DataType = "string"
	DataTypeNumber DataType = "float64"
	DataTypeBool   DataType = "bool"
	DataTypeNull   DataType = "null"
	DataTypeArray  DataType = "array"
	DataTypeMap    DataType = "map"
)

type ESP interface {
	Configure(*Configuration) Configuration
	GetCurrentState() ([]byte, error)
}

// Configuration flags for this package.
type Configuration struct {
	RemoveNonExistantElementIsError      bool // Set to TRUE if trying to remove a non-existent element should throw an error
	RemoveNonExistantArrayElementIsError bool // Set to TRUE if trying to remove a non-existent array element should throw an error
}

// Local config defaults
var config = Configuration{
	RemoveNonExistantElementIsError:      true,  // Default = throw error if removing non-existent element
	RemoveNonExistantArrayElementIsError: false, // Default = don't throw error if removing non-existent array element
}

// Allow the caller to override the configuration
func Configure(configuration *Configuration) Configuration {
	// Change or report the configuration
	if configuration != nil {
		config = Configuration{
			configuration.RemoveNonExistantElementIsError,
			configuration.RemoveNonExistantArrayElementIsError,
		}
	}
	return config
}

// Package-local regex for finding array indicies in paths
var arrayRegex = regexp.MustCompile(`\[([^\[\]]*)\]`)

// documentMap is an internal structure used to hold a json object. Each element is a named property.
type documentMap struct {
	IsArray  bool                        `json:",omitempty"`
	Elements map[string]*documentElement `json:",omitempty"`
}

// documentElement can be any one of: A named property; a named array; an anonymous array; or a named sub-object.
type documentElement struct {
	Name         string             `json:",omitempty"`
	ElementType  DataType           `json:",omitempty"`
	Value        string             `json:",omitempty"` // For properties
	Content      *documentMap       `json:",omitempty"` // For sub-objects
	ArrayContent []*documentElement `json:",omitempty"` // For arrays
}

// GetCurrentState takes a source document object, containing a base document and a sequence of zero or more events.
//
//	It applies each event in turn to the base document, and returns the resulting final document, which will
//	represent the current state of the object, at the point it was loaded.
func (doc Document) GetCurrentState() ([]byte, error) {
	// Map, apply, build, return...
	docMap, err := makeMap(doc.BaseDocument)
	if err != nil {
		return nil, err
	}

	err = docMap.applyEvents(doc)
	if err != nil {
		return nil, err
	}

	return docMap.buildResult()
}

/*
	The following functions are all helpers to enable GetCurrentState to do it's thing.
*/

// makeMap generates a "virtual DOM" view of the document. This makes it far easier than trying to
// muck around with the actual document object  using reflection.
func makeMap(document []byte) (*documentMap, error) {
	// Print an analysis of the document using reflection

	// Unmarshal the document ready for reflection
	var unmarshalledDocument interface{}
	err := json.Unmarshal(document, &unmarshalledDocument)
	if err != nil {
		// Unmarshalling error, do something here
		return nil, err
	}

	// Scan for elements using reflection
	baseDocVal := reflect.ValueOf(unmarshalledDocument)
	switch baseDocVal.Kind() {
	case reflect.Map:
		return mapMapElems(baseDocVal), nil
	case reflect.Slice:
		// We have to return a document map; so use a magic variable as an array "holder"
		// This will be removed when the document is rebuilt.
		// This is only needed at the root level
		return &documentMap{
			IsArray: true,
			Elements: map[string]*documentElement{
				"array": {
					ElementType:  "array",
					ArrayContent: mapSliceElems(baseDocVal),
				},
			},
		}, nil
	}

	return nil, errors.New("base document must have a Kind of reflect.Map or reflect.Slice")
}

// mapMapElems recursively maps json objects from the document, using reflection
func mapMapElems(inputMap reflect.Value) *documentMap {
	// Create an output map to return
	outMap := documentMap{
		Elements: make(map[string]*documentElement),
	}

	iter := inputMap.MapRange()
	for iter.Next() {
		switch iter.Value().Elem().Kind() {
		case reflect.Slice: // Arrays/Slices are both treated as arrays
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:         iter.Key().String(),
				ElementType:  DataTypeArray,
				ArrayContent: mapSliceElems(iter.Value().Elem()),
			}
		case reflect.Map: // A map would be a sub-object with fields/array content
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:        iter.Key().String(),
				ElementType: DataTypeMap,
				Content:     mapMapElems(iter.Value().Elem()),
			}
		case reflect.Invalid: // uh-oh...
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:        iter.Key().String(),
				ElementType: DataTypeNull,
				Value:       "", // Null values have no value (erm, obviously?)
			}
		case reflect.Float64:
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:        iter.Key().String(),
				ElementType: DataType(iter.Value().Elem().Kind().String()),
				Value:       strconv.FormatFloat(iter.Value().Elem().Float(), 'f', -1, 64),
			}
		case reflect.Bool:
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:        iter.Key().String(),
				ElementType: DataType(iter.Value().Elem().Kind().String()),
				Value:       strconv.FormatBool(iter.Value().Elem().Bool()),
			}

		default: // Anything else is just a key:value property
			outMap.Elements[iter.Key().String()] = &documentElement{
				Name:        iter.Key().String(),
				ElementType: DataType(iter.Value().Elem().Kind().String()),
				Value:       iter.Value().Elem().String(),
			}
		}
	}

	return &outMap
}

// mapSliceElems recursively maps json arrays in the document, using reflection
func mapSliceElems(theSlice reflect.Value) []*documentElement {
	outSlice := make([]*documentElement, 0)
	for i := 0; i < theSlice.Len(); i++ {
		switch theSlice.Index(i).Elem().Kind() {
		case reflect.Map:
			outSlice = append(outSlice, &documentElement{
				ElementType: "map",
				Content:     mapMapElems(theSlice.Index(i).Elem()),
			})
		case reflect.Slice:
			outSlice = append(outSlice, &documentElement{
				ElementType:  "array",
				ArrayContent: mapSliceElems(theSlice.Index(i).Elem()),
			})
		default:
			outSlice = append(outSlice, &documentElement{
				ElementType: DataType(theSlice.Index(i).Elem().Kind().String()),
				Value:       theSlice.Index(i).Elem().String(),
			})
		}
	}

	return outSlice
}

// applyEvents takes each event in the collection in turn, and applies it cumulatively, instruction-by-instruction, to the document.
func (docMap *documentMap) applyEvents(document Document) error {
	// Apply any events to the documentMap to create our new document.
	for _, event := range document.Events {
		// Events have instructions - follow each instruction in the event
		for _, instruction := range event.Instructions {

			var err error
			// Special cases: Path = "" and DataType = Array or Map and ActionType != remove THEN replace base doc with instruction value
			if instruction.Path == "" && (instruction.DataType == DataTypeArray || instruction.DataType == DataTypeMap) && instruction.ActionType != ActionTypeRemove {
				// Replacement time
				var newDocMap *documentMap
				newDocMap, err = docMap.replace(instruction)
				if newDocMap != nil {
					docMap.Elements = newDocMap.Elements
					docMap.IsArray = newDocMap.IsArray
				}
			} else {
				// All remaining use cases
				switch instruction.ActionType {
				case ActionTypeSetOrAdd:
					err = docMap.setOrAdd(instruction)
				case ActionTypeSetOnly:
					err = docMap.setOnly(instruction)
				case ActionTypeAddOnly:
					err = docMap.addOnly(instruction)
				case ActionTypeRemove:
					err = docMap.removeElement(instruction)
				default:
					err = fmt.Errorf("unexpected instruction action type `%s`", instruction.ActionType)
				}
			}
			if err != nil {
				return err
			}

		}
	}

	// Success!
	return nil
}

// buildResult - Takes the finalised document map, and builds it into a JSON object, ready for sending back to the consumer.
func (docMap *documentMap) buildResult() ([]byte, error) {
	// Re-create the original document from the map
	bareDocument, err := buildMap(docMap)
	if err != nil {
		return nil, err
	}

	if docMap.IsArray {
		return []byte(bareDocument), nil
	} else {
		return []byte(fmt.Sprintf("{%s}", bareDocument)), nil
	}
}

/*
	The following functions make actual changes to the document map, based on the instruction's actiontype property
	TODO: Implement addOnly
*/

// setOrAdd locates the element to be set - creating it, and the path to it, if necessary - then sets the value.
func (docMap *documentMap) setOrAdd(instruction EventInstruction) error {
	// Locate the element to modify/add
	elem, err := getMapPathElement(instruction.Path, true, docMap)
	if err != nil {
		return err
	}

	// ...then modify it
	return elem.setValue(instruction.DataType, instruction.Value)
}

// setOrAdd locates the element to be set, then sets the value.
// If the element doesn't exist (or any part of the path to it doesn't exist), it errors.
func (docMap *documentMap) setOnly(instruction EventInstruction) error {
	// Locate the element to modify
	elem, err := getMapPathElement(instruction.Path, false, docMap)
	if err != nil {
		return err
	}

	// ...then modify it
	return elem.setValue(instruction.DataType, instruction.Value)
}

// addOnly locates the PARENT of the element to set; then checks to see if the property exists or not.
// If it doesn't exist, it adds it; if it does exist, it errors.
func (docMap *documentMap) addOnly(instruction EventInstruction) error {
	return errors.New("addOnly not implemented")
}

// replace takes the entire document, throws it away, and replaces it with the
// supplied value.
// Use case: Create a base document from an array or map value.
// Therefore: Throw error if base doc is not an empty object.
func (docMap *documentMap) replace(instruction EventInstruction) (*documentMap, error) {
	if len(docMap.Elements) > 0 {
		return nil, errors.New("invalid instruction - can't replace non-empty base document")
	}
	newDocMap, err := makeMap([]byte(instruction.Value))
	if err != nil {
		return nil, fmt.Errorf("invalid instruction - new base document is not valid: %w", err)
	}
	return newDocMap, nil
}

// remove locates an element and, if successful, deletes it from the map.
func (docMap *documentMap) removeElement(instruction EventInstruction) error {
	// Locate the element's parent...
	parentPathParts := strings.Split(instruction.Path, ".")
	lastPath := parentPathParts[len(parentPathParts)-1]

	// Looking at the last part of the path... if it's an array indexer, then just strip the indexer & return the entire array.
	// if it's just a name, then drop it from the path entirely.
	// If there's no path left, then fine, we're at the right level already...
	if strings.Contains(lastPath, "[") {
		// Array Indexer... dump the outermost one & return the property (and any remaining nest levels) to the path
		matchArrays := arrayRegex.FindAllString(lastPath, -1)
		parentPathParts[len(parentPathParts)-1] = strings.TrimSuffix(lastPath, matchArrays[len(matchArrays)-1])
		lastPath = matchArrays[len(matchArrays)-1]
	} else {
		// Normal property/map property: So rebuild the path without it
		parentPathParts = parentPathParts[:len(parentPathParts)-1]
	}

	// Go find the parent path element...
	parentPath := strings.Join(parentPathParts, ".")
	parentElem, err := getMapPathElement(parentPath, false, docMap)
	if err != nil {
		if config.RemoveNonExistantElementIsError {
			return err
		}
		return nil
	}

	// Find the lastpath element in parentElem, and remove it.
	if strings.HasPrefix(lastPath, "[") {
		// Is an array element...
		arrayIndex := strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(lastPath, "]"), "["))

		// Check to see if the array isn't empty first... (unless arrayIndex=all)
		if arrayIndex != "all" && len(parentElem.ArrayContent) == 0 {
			if config.RemoveNonExistantArrayElementIsError {
				return errors.New("attempt to remove array element failed, array was empty")
			}
		}
		switch arrayIndex {
		case "all":
			parentElem.ArrayContent = []*documentElement{} // Clear the entire array
		case "first":
			parentElem.ArrayContent = parentElem.ArrayContent[1:] // Take out the first item only
		case "last":
			parentElem.ArrayContent = parentElem.ArrayContent[:len(parentElem.ArrayContent)-1] // Take out the last item only
		default:
			return fmt.Errorf("`%s` is not a supported array index for the remove action", arrayIndex)
		}
	} else {
		for k := range parentElem.Content.Elements {
			if strings.EqualFold(lastPath, k) {
				// gotcha.
				delete(parentElem.Content.Elements, k)
				return nil
			}
		}
	}

	// Element didn't exist in parent. Is this an error?
	if config.RemoveNonExistantElementIsError {
		return fmt.Errorf("element `%s` not found when trying to remove it", lastPath)
	}

	// Element didn't exist, but that's not an error.
	return nil
}

// setValue overwrites a documentElement's datatype & value. It is used by all the setters.
func (elem *documentElement) setValue(dataType DataType, value string) error {
	elem.ElementType = dataType
	switch dataType {
	// First three are basic "set the value" types
	case DataTypeString:
		fallthrough
	case "float64":
		fallthrough
	case "bool":
		elem.Value = value
	case "null":
		elem.Value = ""
	case "map":
		// Decode the instruction value JSON & then apply the map
		patchMap, err := makeMap([]byte(value))
		if err != nil {
			// Unmarshalling error, do something here
			log.Printf("error unmarshalling instruction value `%s`: %v", value, err)
			return err
		}

		elem.Content = patchMap // That was easier than expected...

	case "array":
		// Decode as above. This should be an array...
		patchMap, err := makeMap([]byte(value))
		if err != nil {
			// Unmarshalling error, do something here
			log.Printf("error unmarshalling instruction value `%s`: %v", value, err)
			return err
		}
		elem.ArrayContent = patchMap.Elements["array"].ArrayContent // Need to work on this one...

	}

	return nil
}

/*
	The following two functions recursively locate an item, either by an array indexer, or based purely on a path
	e.g. Prop1.SubProp1.SubSubProp1[first].ArrayProp1 will hunt through the document map to find the ArrayProp1 element,
		which will be in the first element of the array, which itself is a property called SubSubProp1 in object SubProp1
		which is a property of Prop1 of the root document...(!) See the README.md file for formatted examples.
	//TODO: introduce more flexible array indexers, e.g. conditionals as well as first, last & new
*/

func getArrayPathElement(arrayActions, basePath string, createIfMissing bool, rootElements *[]*documentElement) (*documentElement, error) {
	// Use a regex to get all [x][y][z] patterns out of arrayActions
	matchArrays := arrayRegex.FindAllString(arrayActions, -1)
	if len(matchArrays) == 0 {
		// Oops
		panic(fmt.Sprintf("attempting to get array action from `%s`, regex failed", arrayActions))
	}
	arrayAction := strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(matchArrays[0], "]"), "["))
	nextAction := ""
	if len(matchArrays) > 1 {
		nextAction = strings.Join(matchArrays[1:], "")
	}

	switch arrayAction {
	case "first":
		// Find the first array element. Add a new one if createIfMissing is set.
		if len(*rootElements) > 0 {
			if nextAction != "" {
				// Nested array, move on to the next level
				return getArrayPathElement(nextAction, basePath, createIfMissing, &(*rootElements)[0].ArrayContent)
			}
			// Found the item. Is this a plain value array?
			if basePath != "" {
				// Nope - continue traversing
				return getMapPathElement(basePath, createIfMissing, (*rootElements)[0].Content)
			}
			// Yes; so return it
			return (*rootElements)[0], nil
		} else if !createIfMissing {
			// If createIfMissing is NOT set, then abandon.
			return nil, errors.New("empty array encountered when seeking first element")
		}
		// Otherwise, fall-through into the append new item code.
		fallthrough
	case "new":
		// Create a new array element.
		if nextAction != "" {
			// Nested array... go on then
			*rootElements = append(*rootElements, &documentElement{
				ElementType:  "array",
				ArrayContent: make([]*documentElement, 0),
			})
			return getArrayPathElement(nextAction, basePath, createIfMissing, &(*rootElements)[0].ArrayContent)
		}
		if basePath != "" {
			// Needs a map.
			*rootElements = append(*rootElements, &documentElement{
				ElementType: "map",
				Content: &documentMap{
					Elements: make(map[string]*documentElement),
				},
			})
			return getMapPathElement(basePath, createIfMissing, (*rootElements)[0].Content)
		}
		// Must be a value, then.
		*rootElements = append(*rootElements, &documentElement{
			ElementType: "null", // We don't know what's going in here
		})
		// Return the new item
		return (*rootElements)[len(*rootElements)-1], nil
	case "last":
		// Find the last array element. Do NOT add a new one, in this case
		return nil, errors.New("last array element is not yet supported")
	default:
		// Unsupported, whatever it is.
		return nil, fmt.Errorf("array element operator `%s` is not supported", arrayAction)
	}
}

func getArrayIndexer(pathPart string) (string, string) {
	// Split the [x] bit off a named array property. [x][y] (to any level of nesting) is handled.

	// Split on first "[", and set up the array finder.
	nameAndArrayIndex := strings.SplitN(pathPart, "[", 2)
	pathPart = nameAndArrayIndex[0]
	arrayPart := fmt.Sprintf("[%s", nameAndArrayIndex[1])

	// Return the output (e.g. "NestedArray", "[x][y][z]")
	return pathPart, arrayPart
}

func getMapPathElement(basePath string, createIfMissing bool, startAt *documentMap) (*documentElement, error) {
	// Decompose the path into elements, then navigate the map to find the entry point for our delta.
	// Note that we have to start at a map; so this won't work where the initial path is an array element (TODO)
	// If we end up at a dead end, either create a new element (if createIfMissing is true) or abort with an error.
	pathParts := strings.Split(basePath, ".")
	nextPath := ""
	if len(pathParts) > 1 {
		nextPath = strings.Join(pathParts[1:], ".") // Rebuild the rest of the path for the next call.
	}

	findElementWithName := strings.ToLower(pathParts[0])

	// Check for arrays...
	seekArray := strings.Contains(findElementWithName, "[")
	arrayElement := ""
	if seekArray {
		findElementWithName, arrayElement = getArrayIndexer(findElementWithName)
	}

	for _, elem := range startAt.Elements {
		if strings.ToLower(elem.Name) == findElementWithName {
			// Gotcha!
			if seekArray {
				// Expected element is an array... so jump into the array element handler.
				return getArrayPathElement(arrayElement, nextPath, createIfMissing, &elem.ArrayContent)
			}
			// If element contains sub-elements, do we need to drill down?
			if elem.ElementType == "map" && nextPath != "" {
				return getMapPathElement(nextPath, createIfMissing, elem.Content)
			}

			if elem.ElementType == "null" && nextPath != "" {
				// We've reached a NULL, but there's more to the path...
				// Therefore we must be creating a new map...
				if createIfMissing {
					elem.ElementType = "map"
					elem.Content = &documentMap{
						Elements: make(map[string]*documentElement),
					}
					return getMapPathElement(nextPath, createIfMissing, elem.Content)
				} else {
					// Can't go on.
					return nil, fmt.Errorf("encountered null value in path, and create path is not enabled")
				}
			}

			// If we get here, then we found the element and we don't need to drill any further 'cos nextPath is ""
			return elem, nil

		}
	}

	// We failed to find the element. So create it if needed...
	if createIfMissing {
		if seekArray {
			startAt.Elements[pathParts[0]] = &documentElement{
				Name:         pathParts[0],
				ElementType:  "array",
				ArrayContent: make([]*documentElement, 0),
			}
			return getArrayPathElement("", nextPath, createIfMissing, &startAt.Elements[pathParts[0]].ArrayContent)
		}

		if nextPath != "" {
			// Create a new map element here, and move on
			startAt.Elements[pathParts[0]] = &documentElement{
				Name:        pathParts[0],
				ElementType: "map",
				Content: &documentMap{
					Elements: make(map[string]*documentElement),
				},
			}
			return getMapPathElement(nextPath, createIfMissing, startAt.Elements[pathParts[0]].Content)
		}

		// If there's no path left, we've reached the end of our search (hurrah!) Return the parent element.
		startAt.Elements[pathParts[0]] = &documentElement{
			Name:        pathParts[0],
			ElementType: "null", // We don't know what's going in it...
		}
		return startAt.Elements[pathParts[0]], nil

	}

	return nil, fmt.Errorf("unable to locate element named `%s` and createIfMissing is false", pathParts[0])
}

/*
	The next two functions recursively (between them) traverse the document, building each object from the bottom up as
	a valid JSON string; eventually the topmost call will return a complete (except for the overall curly braces, or
	square brackets as applicable) valid JSON object, ready to go back to the caller.
*/

func buildArray(arrayContent []*documentElement) (string, error) {
	// Special case for empty arrays
	if len(arrayContent) == 0 {
		return "", nil
	}

	// Otherwise iterate over the array and build as appropriate
	newArray := "%s"
	for _, v := range arrayContent {
		switch v.ElementType {
		case DataTypeArray:
			// Add an array item
			subst, err := buildArray(v.ArrayContent)
			if err != nil {
				return "", err
			}
			newArray = strings.Replace(newArray, "%s", fmt.Sprintf(`[%s],%%s`, subst), -1)
		case DataTypeMap:
			// Add a sub-object
			subst, err := buildMap(v.Content)
			if err != nil {
				return "", err
			}
			newArray = strings.Replace(newArray, "%s", fmt.Sprintf(`{%s},%%s`, subst), -1)
		case DataTypeString:
			// Add a string property
			newArray = strings.Replace(newArray, "%s", fmt.Sprintf(`"%s",%%s`, escapeString(v.Value)), -1)
		case DataTypeNumber:
			// Add a numeric property
			newArray = strings.Replace(newArray, "%s", fmt.Sprintf(`%s,%%s`, v.Value), -1)
		case DataTypeBool:
			// Add a boolean property
			newArray = strings.Replace(newArray, "%s", fmt.Sprintf(`%s,%%s`, v.Value), -1)
		case DataTypeNull:
			// Add a named null property
			newArray = strings.Replace(newArray, "%s", "null,%s", -1)
		default:
			return "", fmt.Errorf("unexpected data type `%s` found in document array", v.ElementType)
		}
	}

	// Truncate the trailing ",%s" and return
	return strings.TrimSuffix(newArray, ",%s"), nil
}

func buildMap(docMap *documentMap) (string, error) {
	// Special case for empty maps
	if len(docMap.Elements) == 0 {
		return "", nil
	}

	// Otherwise iterate over the properties & set them as appropriate
	newMap := "%s"
	for k, v := range docMap.Elements {
		switch v.ElementType {
		case DataTypeArray:
			// Add an array item
			subst, err := buildArray(v.ArrayContent)
			if err != nil {
				return "", err
			}
			if docMap.IsArray {
				// Special case if root map has "IsArray" set
				newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`[%s],%%s`, subst), -1)
			} else {
				newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":[%s],%%s`, k, subst), -1)
			}
		case DataTypeMap:
			// Add a sub-object
			subst, err := buildMap(v.Content)
			if err != nil {
				return "", err
			}
			newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":{%s},%%s`, k, subst), -1)
		case DataTypeString:
			// Add a string property
			newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":"%s",%%s`, k, escapeString(v.Value)), -1)
		case DataTypeNumber:
			// Add a numeric property
			newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":%s,%%s`, k, v.Value), -1)
		case DataTypeBool:
			// Add a boolean property
			newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":%s,%%s`, k, v.Value), -1)
		case DataTypeNull:
			// Add a named null property
			newMap = strings.Replace(newMap, "%s", fmt.Sprintf(`"%s":null,%%s`, k), -1)
		default:
			// Unexpected data type - error
			return "", fmt.Errorf("unexpected data type `%s` found in document map", v.ElementType)
		}
	}

	// Truncate the trailing ",%s" and return
	return strings.TrimSuffix(newMap, ",%s"), nil
}

// escapeString simply returns a string value, with internal quotes and backslashes escaped with backslashes; for use when the source jSON value
// contained a string containing quotes and/or backslashes (e.g. a JSON string)
func escapeString(input string) string {
	// Escape backslashes and quotes in a string value
	return strings.Replace(
		strings.Replace(
			input,
			"\\",
			"\\\\",
			-1,
		),
		"\"",
		"\\\"",
		-1,
	)
}
