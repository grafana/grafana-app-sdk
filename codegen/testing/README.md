# Testing files

The contents of [golden_generated] are expected output for inputs of `customKind` and `customKind2`. 
The way these objects can be defined depends on the parser, but any shared output (for example, go types) 
should always be the same regardless of parser, as they use the same shared jennies.

There is no helpful utility to re-create the golden files.  Either manually update them or setup a parallel
project and copy the cue files manually.