# DOCUMENTS - and how to modify them

## Introduction

This package implements an Event Sourcing data model; specifically, one which can take any raw document (in JSON format), 
a collection of "events", each of which contains a collection of "instructions", which describe what to do to the document.

## Document

The "Document" is the root level object. It has a "base document" - which could be as simple as an empty object, i.e. {}; or which could 
be a fairly complex document in its own right. The "base document" represents the most recent snapshot of an object state.

## DocumentEvent

DocumentEvent is an array of instructions which, together, represent an Event. All EventInstructions in a DocumentEvent must be applied 
successfully, for the event to be considered successful.

## EventInstruction

Each DocumentEvent is a collection of event instructions. These may be in any order, as they will all be applied to a document together; and 
generally should not have any internal dependencies on each other.

Each instruction can act on a single part of the document. That part might be:
- An object
- An array
- A property
- The entire document (only for empty documents, and only if the instruction type is a map or array)

An instruction contains:
- a dot-separated path to the property (e.g. `FirstObject.SecondObject.FieldName`)
- a `Value` (except for "remove" instructions)
- a `DataType` (except for "remove" instructions) which tells the system what to do with the value:
- - one of `string`, `float64` or `bool`: For basic data types
- - `map`: To indicate the value property contains a JSON-encoded object, or
- - `array`: TO indicate the value property contains a JSON-encoded array
- an `ActionType`, which determines what this instruction is:
- - `SetOrAdd`: Will set the named property (or array element) to the supplied value; adding both the path to it, and the property itself, if needed.
- - `SetOnly`: Will update an existing named property (or array element) to the supplied value; it will throw an error if the property doesn't already exist
- - `AddOnly`: As `SetOnly`, except the property must NOT exist in advance. (__TODO__ Not implemented.)
- - `Remove`: Will delete the named property, or array element (or the entire array). Value and DataType are ignored.


There are three kinds of structure which the system can handle. Two of these are refreshingly straightforward, and one is mind-bendingly complicated.

### Ordinary Fields.

Ordinary fields are the easiest things to reason about. Fortunately. For example, consider the following json:

```
{
    "Field1": "Value1",
    "Field2": "Value2"
}
```

Let's say we wanted to add `"Field3": "Value3"` to this structure... our event instruction would be very easy:

```
{
    "Path": "Field3",
    "DataType": "string",
    "ActionType": "SetOrAdd",    
    "Value": "Value3"
}
```


After we apply this instruction, our document looks like this:
```
{
    "Field1": "Value1",
    "Field2": "Value2",
    "Field3": "Value3"
}
```

### Map fields (complex objects)
We could add a more complex field. For example:

```
{
    "Path": "Field4",
    "DataType": "map",
    "ActionType": "SetOrAdd",
    "Value": "{\"Field4_1\":\"Value4.1\",\"Field4_2\":\"Value4.2\"}"
}
```
Note that the data type is "map" to indicate that "Value" contains a JSON-formatted object. The source document is shown as an escaped string, in reality
this would be the byte array output from json.Marshal, for example.

Now our document looks like this:
```
{
    "Field1": "Value1",
    "Field2": "Value2",
    "Field3": "Value3",
    "Field4": {
        "Field4_1": "Value4.1",
        "Field4_2": "Value4.2"
    }
}
```

Let's update Field4_2 to have a new value:

```
{
    "Path": "Field4.Field4_2",
    "DataType": "string",
    "ActionType": "SetOrADd",    
    "Value": "Value4.2_modified"
}
```

...and get rid of Field4_1 completely:

```
{
    "Path": "Field4.Field4_1",
    "ActionType": "Remove"
}
```

Now our object looks like this:
```
{
    "Field1": "Value1",
    "Field2": "Value2",
    "Field3": "Value3",
    "Field4": {
        "Field4_2": "Value4.2_modified"
    }
}
```

### Arrays

Manipulating arrays is a little more difficult than the above, mainly because there's no guarantee that an array, when serialised or
deserialised, will maintain its order.

Furthermore, we need to be able to add, remove or modify array elements - and any array element can be, well anything: A number, a string,
an object, another array, or a null. Arrays don't even have to hold the same kind of element! Array[0] could be a string, Array[1] a number,
Array[2] another array and Array[3] an object!

In order to not go loopy, only a limited number of array operators are currently supported. More may be added as time/need permits.

First, let's take an example base document:

```
[
    {
        "arrayObjectId": 0,
        "arrayObjectName": "array-object-0"
    },
    {
        "arrayObjectId": 1,
        "arrayObjectName": "array-object-1"
    }
]
```
A simple array, containing two elements, each of which is an object following the same pattern.

We can add a new element with an instruction like this:
{
    "Path": "[new]",
    "DataType": "map",
    "ActionType": "SetOrAdd",
    "Value": "{\"arrayObjectId\":2,\"arrayObjectName\":\"array-object-2\"}"
}

If the array is embedded within a JSON object, e.g.:

```
{
    "JsonProperty":[
        "Array1",
        "Array2",
        "etc.
    ]
}
```

then the path should include the name of the property, i.e. `JsonProperty[new]`

The part inside the square brackets is the `Indexer`; and the following are currently supported:
- `[new]` - Creates a new element based on Value. Not valid for `SetOnly` or `Remove` operations
- `[first]` - References the first element in an array. Will create it if `SetOrAdd` and the array is empty, or remove it for `Remove` instructions. `AddOnly` will throw an error if an array element already exists.
- `[last]` - As `[first]`, but with the last element in an array. `AddOnly` will throw an error, unless the array is empty.
- `[all]` - Will empty an array completely (`Remove` only); not valid for any other operation.

__TODO__: Add a `[cond]` indexer, which will use a condition path/value to locate element(s) in an array.
