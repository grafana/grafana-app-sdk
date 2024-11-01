# Testing files

The contents of [golden_generated] are expected output for inputs of `customKind` and `customKind2`. 
The way these objects can be defined depends on the parser, but any shared output (for example, go types) 
should always be the same regardless of parser, as they use the same shared jennies.

To re-generate the test files from the current state of the project, run `make regenerate-codegen-test-files`.