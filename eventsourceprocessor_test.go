package eventsourceprocessor_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	eventsourceprocessor "github.com/adev73/event-source-processor"
	"github.com/stretchr/testify/assert"
)

const debugOutput = true // If true, will print input & output docs to console.

func TestBaseDocument(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestBaseDocument", "base.json", nil)
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Sample some properties to make sure the document built correctly
	that.Contains(string(outputDoc), `"objectName":"object-name"`)
	that.Contains(string(outputDoc), `"nullField":null`)
}

func TestAddEvent1(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent1", "base.json", []string{"event1.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 1
	that.Contains(string(outputDoc), `"newFieldFromEvent1":"Event 1 adds this field"`)
	that.Contains(string(outputDoc), `"stringField":"Event 1 replaces this field"`)
	that.Contains(string(outputDoc), `"newNullField":null`)
}

func TestAddEvent1and2(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent1", "base.json", []string{"event1.json", "event2.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 1
	that.Contains(string(outputDoc), `"newFieldFromEvent1":"Event 1 adds this field"`)
	that.Contains(string(outputDoc), `"stringField":"Event 1 replaces this field"`)
	// Event 2
	that.Contains(string(outputDoc), `"newObjectFieldFromEvent2":"This should be added to objectField by Event 2"`)
	that.Contains(string(outputDoc), `"event2objectfield1":"This object is added from Event 2"`)
}

func TestAddEvent1and2and3(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent1", "base.json", []string{"event1.json", "event2.json", "event3.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 1
	that.Contains(string(outputDoc), `"newFieldFromEvent1":"Event 1 adds this field"`)
	that.Contains(string(outputDoc), `"stringField":"Event 1 replaces this field"`)
	// Event 2
	that.Contains(string(outputDoc), `"newObjectFieldFromEvent2":"This should be added to objectField by Event 2"`)
	that.Contains(string(outputDoc), `"event2objectfield1":"This object is added from Event 2"`)
	// Event 3
	that.Contains(string(outputDoc), `"arrayObjectId":1`) // Was already there
	that.Contains(string(outputDoc), `"arrayObjectId":2`) // ...should have been added.
}

func TestBaseDocArray(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestBaseDocArray", "baseArray.json", nil)
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	that.Contains(string(outputDoc), `"arrayObjectId":0`)
	that.Contains(string(outputDoc), `"arrayObjectId":1`)
}

func TestBaseDocNestedArray(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestBaseDocArray", "baseNestedArray.json", nil)
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	that.Contains(string(outputDoc), `"arrayObjectName":"array-object-0.0"`)
	that.Contains(string(outputDoc), `"arrayObjectName":"array-object-1.1"`)
	that.Contains(string(outputDoc), `["valueArray-0","valueArray-1"]`)
}

func TestAddEvent1through4(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent1", "base.json", []string{"event1.json", "event2.json", "event3.json", "event4.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 1
	that.Contains(string(outputDoc), `"newFieldFromEvent1":"Event 1 adds this field"`)
	that.Contains(string(outputDoc), `"stringField":"Event 1 replaces this field"`)
	// Event 2 (these were subsequently removed by Event 4)
	that.NotContains(string(outputDoc), `"newObjectFieldFromEvent2"`)
	that.NotContains(string(outputDoc), `"event2objectfield1"`)
	// Event 3
	that.Contains(string(outputDoc), `"arrayObjectId":1`) // Was already there
	that.Contains(string(outputDoc), `"arrayObjectId":2`) // ...should have been added.
}

func TestAddEvent5(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent5", "base.json", []string{"event5.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 5
	that.Contains(string(outputDoc), `"newObject":{`)
	that.Contains(string(outputDoc), `"newSubObject":{`)
	that.Contains(string(outputDoc), `"newSubObjectString":"Event 5 adds newObject, containing newSubObject, with a string field newSubSubObject set to this value"`)
}

func TestAddEvent6aToEmptyObject(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent6a", "emptyBase.json", []string{"event6a.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 6a
	that.Contains(string(outputDoc), `"id":"some-uuid-we-generated"`)
	that.Contains(string(outputDoc), `"brandName":"someBrand"`)
	that.Contains(string(outputDoc), `"hotelName":"Some Hotel"`)
}

func TestAddEvent6bToEmptyObject(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent6b", "emptyBase.json", []string{"event6b.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 6b
	that.NotContains(string(outputDoc), `{"":{`) // Does not contain an un-named sub object
	that.Contains(string(outputDoc), `"id":"some-uuid-we-generated"`)
	that.Contains(string(outputDoc), `"brandName":"somebrand"`)
	that.Contains(string(outputDoc), `"hotelName":"Some Hotel"`)
}

func TestAddEvent6bToNotEmptyObject_Fails(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent6", "base.json", []string{"event6b.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.NotNil(err)
	// Event 6a
}

func TestAddEvent6cToEmptyObject(t *testing.T) {
	that := assert.New(t)
	inputDoc := buildDocument("TestAddEvent6c", "emptyBase.json", []string{"event6c.json"})
	outputDoc, err := inputDoc.GetCurrentState()
	prettyPrintObject("Input Document", inputDoc)
	prettyPrint("Output Document", outputDoc)

	// Nothing unexpected went wrong
	that.Nil(err)
	// Event 6c
	that.Equal(string(outputDoc), `["ArrayElement1","ArrayElement2"]`)
}

// Helper functions
func prettyPrint(description string, doc []byte) {
	if !debugOutput {
		return
	}
	// Convert byte array JSON document into a pretty version & print it
	var intermediateDoc interface{}
	json.Unmarshal(doc, &intermediateDoc)
	result, _ := json.MarshalIndent(intermediateDoc, "", "  ")
	fmt.Printf("%s\n--------------------\n%s--------------------\n", description, string(result))
}

func prettyPrintObject(description string, doc interface{}) {
	if !debugOutput {
		return
	}
	// Convert byte array JSON document into a pretty version & print it
	result, _ := json.MarshalIndent(doc, "", "  ")
	fmt.Printf("%s\n--------------------\n%s--------------------\n", description, string(result))
}

func buildDocument(caller string, baseFile string, eventFiles []string) eventsourceprocessor.Document {
	baseDocument, err := loadFile(fmt.Sprintf("./test_data/%s", baseFile))
	if err != nil {
		panic(fmt.Sprintf("failed to load base document data in %s", caller))
	}
	theDocument := eventsourceprocessor.Document{
		BaseDocument: baseDocument,
		Events:       []eventsourceprocessor.DocumentEvent{},
	}

	for _, eventFile := range eventFiles {
		instructionSource, err := loadFile(fmt.Sprintf("./test_data/%s", eventFile))
		if err != nil {
			panic(fmt.Sprintf("failed to load event file %s in %s", eventFile, caller))
		}
		var instructions []eventsourceprocessor.EventInstruction
		err = json.Unmarshal(instructionSource, &instructions)
		if err != nil {
			panic(fmt.Sprintf("failed to decode event file %s in %s", eventFile, caller))
		}
		theDocument.Events = append(theDocument.Events, eventsourceprocessor.DocumentEvent{
			Instructions: instructions,
		})
	}
	return theDocument
}

func loadFile(fileName string) ([]byte, error) {
	// Attempt to load the file. If we faile, return an error
	return os.ReadFile(fileName)
}
