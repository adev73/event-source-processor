# event-source-processor
A Go library to facilitate event sourcing systems.


# What is it?
eventsourceprocessor is a golang library which builds a JSON document from a "base" document (initial state), by applying
events to it. The output is - assuming all went well - the current (or desired) state of the document.

Obviously, there's a lot more to event sourcing than just building the state... but this is a key component.


# How does it work?
See: Usage.md


# How *well* does it work?
It covers most use cases (that I've thought of) as-is. You can:
- Add or update a field anywhere in the document tree
    - The field can be of string, numeric or boolean type
    - Null values are supported
    - You can also add or replace entire objects or arrays (by encoding them in JSON within the event)
- Remove any field from anywhere in the document tree

Array handling is still a work in progress. You can:
- add new elements
- remove existing elements (first or last, or all)


# Things To Make And Do
The most immediate requirements are:
- Finish up array handling
- Add a condition processor for arrays, to find specific elements
- Much more testing
- Handle properties with dots in the name (don't use these, really, just don't. Your mother wouldn't, and mother knows best.)


# Contributing
Contributions are welcome! Usual procedure - fork, branch, work, PR, review, push :)
