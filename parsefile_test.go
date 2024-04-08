package testtxt

import (
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type test struct {
	title  string
	descr  any
	input  string
	result any
	error  string
}

type descr struct {
	Title string
	Input string
	Count int
	Todo  bool
}

var tests = []test{
	{
		title:  "Minimal input",
		input:  `=TITLE= test`,
		result: &[]descr{{Title: "test"}},
	},
	{
		title: "Only 2 title",
		input: `
=TITLE=t1
=TITLE=t2
`,
		result: &[]descr{{Title: "t1"}, {Title: "t2"}},
	},
	{
		title:  "Trim whitespace from value",
		input:  `=TITLE=  test  abc  `,
		result: &[]descr{{Title: "test  abc"}},
	},
	{
		title: "Set boolean attribute",
		input: `
=TITLE=t1
=TODO=
`,
		result: &[]descr{{Title: "t1", Todo: true}},
	},
	{
		title: "Set boolean attribute, ignore value",
		input: `
=TITLE=t1
=TODO=soon
`,
		result: &[]descr{{Title: "t1", Todo: true}},
	},
	{
		title: "Set number from separate line and directly",
		input: `
=TITLE=t1
=COUNT=
+1234567
=END=

=TITLE=t2
=COUNT=  -654321
`,
		result: &[]descr{
			{Title: "t1", Count: 1234567},
			{Title: "t2", Count: -654321},
		},
	},
	{
		title: "Simple template with substitution",
		input: `
=TEMPL=input
some text
number 42 wins
the end
=END=

=TITLE=t1
=INPUT=[[input]]
=SUBST=/wins/WINS/

=TITLE=t2
=INPUT=
[[input]]++
=SUBST=/42/100/
=SUBST= |the end|/e/|
`,
		result: &[]descr{
			{Title: "t1", Input: "some text\nnumber 42 WINS\nthe end"},
			{Title: "t2", Input: "some text\nnumber 100 wins\n/e/++\n"},
		},
	},
	{
		title: "Simple template in template",
		input: `
=TEMPL=xx
xx
=TEMPL=xxx
[[xx]][[xx]][[xx]]
=TITLE=t1
=INPUT=[[xxx]] [[xx]]
`,
		result: &[]descr{
			{Title: "t1", Input: "xxxxxx xx"},
		},
	},
	{
		title: "Template with array as argument",
		input: `
=TEMPL=xx
{{range . -}}
element={{.}}
{{end}}
=END=

=TITLE=t1
=INPUT=[[xx [a, b, c]]]

=TITLE=t2
=INPUT=
[[xx
- a
- b
- c
]]
=END=
`,
		result: &[]descr{
			{Title: "t1", Input: "element=a\nelement=b\nelement=c\n"},
			{Title: "t2", Input: "element=a\nelement=b\nelement=c\n\n"},
		},
	},
	{
		title: "Call of template is located in brackets",
		input: `
=TEMPL=xx
abc
=TITLE=t1
=INPUT= [[[xx]]] [[[[xx]]]]
`,
		result: &[]descr{
			{Title: "t1", Input: "[abc] [[abc]]"},
		},
	},
	{
		title: "Mixed brackets in YAML and surrounding text",
		input: `
=TEMPL=xx
--{{.}}--
=TITLE=t1
=INPUT= [[[xx [ARG]]]]
`,
		result: &[]descr{
			{Title: "t1", Input: "[--[ARG]--]"},
		},
	},
	{
		title: "Call template with empty parameter",
		input: `
=TEMPL=xx
--{{.}}--
=TITLE=t1
=INPUT= [[xx  ]]
`,
		result: &[]descr{
			{Title: "t1", Input: "--<no value>--"},
		},
	},
	{
		title:  "Title with mixed case",
		descr:  &[]struct{ MixedCase string }{},
		input:  "=MIXED_CASE= test",
		result: &[]struct{ MixedCase string }{{MixedCase: "test"}},
	},

	// Test for errors below.
	{
		title: "No pointer",
		descr: []descr{},
		error: "expecting pointer to empty slice",
	},
	{
		title: "No slice",
		descr: &descr{},
		error: "expecting pointer to empty slice",
	},
	{
		title: "No struct",
		descr: &[]string{},
		error: "expecting slice of struct",
	},
	{
		title: "Empty struct",
		descr: &[]struct{}{},
		error: "expecting struct with at least one field",
	},
	{
		title: "Unexpected type of struct field",
		descr: &[]struct{ Field []string }{},
		input: "=FIELD= test",
		error: `unexpected type "slice" of struct field "Field" in test with =FIELD=test`,
	},
	{
		title: "Unexported title",
		descr: &[]struct{ title string }{},
		input: "=TITLE= test",
		error: "struct field \"title\" must be exported in test with =TITLE=test",
	},
	{
		title: "Empty input",
		error: `missing =TITLE= in first test of file "file"`,
	},
	{
		title: "Empty lines",
		input: `

`,
		error: `missing =TITLE= in first test of file "file"`,
	},
	{
		title: "Only comments",
		input: `#
#
`,
		error: `missing =TITLE= in first test of file "file"`,
	},
	{
		title: "Invalid =SUBST= in preamble",
		input: `=SUBST=/test/foo/`,
		error: `=SUBST= is only valid at bottom of text block in file "file"`,
	},
	{
		title: "Invalid =SUBST= in test",
		input: `
=TITLE=t1
=INPUT=
abc
=END=
=SUBST=/test/foo/`,
		error: "=SUBST= is only valid at bottom of text block in test with =TITLE=t1",
	},
	{
		title: "Missing definition",
		input: `
#
 # comment
==TITLE test`,
		error: `expecting token '=...=' at line 4 of file "file": ==TITLE test`,
	},
	{
		title: "Ignore indented title",
		input: `  =TITLE= test`,
		error: `expecting token '=...=' at line 1 of file "file":   =TITLE= test`,
	},
	{
		title: "Ignore incomplete title",
		input: `=TITLE test`,
		error: `expecting token '=...=' at line 1 of file "file": =TITLE test`,
	},
	{
		title: "Whitespace in name",
		input: `=TI TL E= test`,
		error: `expecting token '=...=' at line 1 of file "file": =TI TL E= test`,
	},
	{
		title: "Invalid name",
		input: `=C++Go= test`,
		error: `expecting token '=...=' at line 1 of file "file": =C++Go= test`,
	},
	{
		title: "Unexpected text after single line input",
		input: `
=TITLE= test
=INPUT=abc
def
`,
		error: `expecting token '=...=' at line 4 of file "file": def`,
	},
	{
		title: "Missing title",
		input: `=INPUT= test`,
		error: `must define =TITLE= before =INPUT= in file "file"`,
	},
	{
		title: "Multiple =INPUT=",
		input: `
=TITLE=t1
=INPUT=
abc
=INPUT=
def
=END=
`,
		error: "found multiple =INPUT= in test with =TITLE=t1",
	},
	{
		title: "Invalid number",
		input: `
=TITLE= t1
=COUNT= two
`,
		error: `invalid value for struct field "Count": strconv.ParseInt: parsing "two": invalid syntax in test with =TITLE=t1`,
	},
	{
		title: "Unexpected attribute",
		input: `
=TITLE= t1
=X= y
`,
		error: "unexpected =X= in test with =TITLE=t1",
	},
	{
		title: "Unexpected attribute in lowercase",
		input: `
=TITLE= test
=input=abc
`,
		error: "unexpected =input= in test with =TITLE=test",
	},
	{
		title: "Missing template name",
		input: `=TEMPL=`,
		error: `missing name after =TEMPL= in file "file"`,
	},
	{
		title: "Invalid template name",
		input: `=TEMPL=++`,
		error: `invalid name after =TEMPL=: "++" in file "file"`,
	},
	{
		title: "Missing template body",
		input: `=TEMPL=xx`,
		error: `missing text after =TEMPL=xx in file "file"`,
	},
	{
		title: "Invalid template body",
		input: `
=TEMPL=xx
abc
=SUBST=//
`,
		error: `invalid substitution: =SUBST=// in file "file"`,
	},
	{
		title: "Calling unknown template",
		input: `
=TITLE=t1
=INPUT= [[xx]]
`,
		error: `calling unknown template "xx" in test with =TITLE=t1`,
	},
	{
		title: "Invalid YAML",
		input: `
=TITLE=t1
=INPUT=
[[xx
- a
- b
--
]]
`,
		error: `invalid YAML data "- a\n- b\n--\n" in call to template "xx":
 "yaml: line 3: could not find expected ':'" in test with =TITLE=t1`,
	},
	{
		title: "Two brackets in YAML not supported",
		input: `
=TEMPL=xx
--{{.}}--
=TITLE=t1
=INPUT= [[[xx [[ARG]]]]]
`,
		error: `invalid YAML data "[[ARG]" in call to template "xx":
 "yaml: line 1: did not find expected ',' or ']'" in test with =TITLE=t1`,
	},
	{
		title: "Error while executing template",
		input: `
=TEMPL=xx
abc{{.part}}def
=TITLE=t1
=INPUT= [[xx xyz]]
`,
		error: `template: xx:1:5: executing "xx" at <.part>: can't evaluate field part in type string in test with =TITLE=t1`,
	},
	{
		title: "Empty substitution",
		input: `
=TITLE=t1
=INPUT=
abc
=SUBST=
`,
		error: `invalid empty substitution in test with =TITLE=t1`,
	},
	{
		title: "Invalid substitution",
		input: `
=TITLE=t1
=INPUT=
abc
=SUBST=/abc/def/i
`,
		error: `invalid substitution: =SUBST=/abc/def/i in test with =TITLE=t1`,
	},
}

func TestParseFile(t *testing.T) {
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			fName := path.Join(workDir, "file")
			if err := os.WriteFile(fName, []byte(test.input), 0644); err != nil {
				t.Fatal(err)
			}
			d := test.descr
			if d == nil {
				d = &[]descr{}
			}
			if err := ParseFile(fName, d); err != nil {
				e := err.Error()
				e = strings.ReplaceAll(e, workDir+"/", "")
				eq(t, test.error, e)
			} else {
				eq(t, test.result, d)
			}
		})
	}
}

func TestTemplateDateFunc(t *testing.T) {
	t.Parallel()
	input := `
=TEMPL=disable_at
disable_at = {{DATE .}}
=TITLE=t1
=INPUT=[[disable_at -10]]
`
	result := &[]descr{{
		Title: "t1",
		Input: "disable_at = " +
			time.Now().AddDate(0, 0, -10).Format("2006-01-02"),
	}}
	d := &[]descr{}
	workDir := t.TempDir()
	fName := path.Join(workDir, "file")
	if err := os.WriteFile(fName, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ParseFile(fName, d); err != nil {
		t.Fatal(err)
	}
	eq(t, result, d)
}

func eq(t *testing.T, expected, got any) {
	if d := cmp.Diff(expected, got); d != "" {
		t.Error(d)
	}
}
