# testtxt

Testtxt is a Go library for table driven tests [^1] with heavy input and
output data. It allows to write table driven tests in test
description files, providing a format that is pleasently human
readable.

The test description file, containing one or several testcases in
testtxt format, is parsed into a slice of test description structs,
that then can be used as a test table.

In testtxt format, inputs, results and other information are indicated with named markers, followed by the actual input, result or info strings:
```
=<MARKER>= <input>
=<MARKER>= <result>
=<MARKER>= <info>
```

With any uppercase string as <MARKER> and any string as <input>.

`Testtxt.ParseFile(file string, l any)` then parses testcases from test
description file into test table `l`.  Test table entries need to be
defined as struct within the test code, with <Marker> being the names
of the struct fields.

If several tests within the test description file require a common
input string, templates can be defined, using the
`=TEMPL=<templateName>` marker.  Templates can then be used behind
any marker `=<MARKER>= [[templateName]]`

If the tool to be tested requires files as input, a utility function
is provided, to create one or multiple files with given input.
Use `testtxt.PrepareFileOrDir(t *testing.T, dst, input string)`
to create an input directory dst and fill it with one or more
input files. Several files are indicated within the input
string by single lines of dashes followed by a filename:
```
--<filename>
<fileinput>
```
If no markers are given, no directory but a single file named dst
is created.

[^1]: <https://go.dev/wiki/TableDrivenTests>
