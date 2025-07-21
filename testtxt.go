package testtxt

/*
   Generate test descriptions from simple text files.

https://github.com/hknutzen/testtxt
   (c) 2024 by Heinz Knutzen <heinz.knutzen@gmail.com>

   This program is free software; you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation; either version 2 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful, but
   WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
   See the GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program; if not, write to the Free Software
   Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA
   02110-1301, USA.
*/

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseFile parses the named file as a list of test descriptions.
func ParseFile(file string, l any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(l)
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("expecting pointer to empty slice")
	}
	v = v.Elem()
	if v.Kind() != reflect.Slice || v.Len() != 0 {
		return fmt.Errorf("expecting pointer to empty slice")
	}
	s := &state{
		src:       data,
		rest:      data,
		templates: make(map[string]*template.Template),
		filename:  file,
		slice:     v,
	}
	return s.parse()
}

type state struct {
	src       []byte
	rest      []byte
	templates map[string]*template.Template
	filename  string
	slice     reflect.Value
}

func (s *state) parse() error {
	el := addElement(s.slice)
	if el.Kind() != reflect.Struct {
		return fmt.Errorf("expecting slice of struct")
	}
	fields := reflect.VisibleFields(el.Type())
	if len(fields) == 0 {
		return fmt.Errorf("expecting struct with at least one field")
	}
	title := toSnakeCase(fields[0].Name)
	var seen map[string]bool
	tVal := ""
	first := true
	for {
		withContext := func(err error) error {
			where := ""
			if tVal != "" {
				where = fmt.Sprintf("in test with =%s=%s", title, tVal)
			} else {
				where = fmt.Sprintf("in file %q", s.filename)
			}
			return fmt.Errorf("%v %s", err, where)
		}
		name, err := s.readDef()
		if err != nil {
			return err
		}
		if name == "" { // EOF
			if first {
				return fmt.Errorf("missing =%s= in first test of file %q",
					title, s.filename)
			}
			return nil
		}
		switch name {
		case "TEMPL":
			if err := s.templDef(); err != nil {
				return withContext(err)
			}
			continue
		case "SUBST":
			return withContext(errors.New(
				"=SUBST= is only valid at bottom of text block"))
		}
		text, err := s.readExpandedText()
		if err != nil {
			return withContext(err)
		}
		if name == title {
			if seen[name] {
				el = addElement(s.slice)
			}
			tVal = text
			first = false
			seen = make(map[string]bool)
		} else if first {
			return withContext(
				fmt.Errorf("must define =%s= before =%s=", title, name))
		}
		if seen[name] {
			return fmt.Errorf(
				"found multiple =%s= in test with =%s=%s", name, title, tVal)
		}
		if err := setVal(el, name, text); err != nil {
			return withContext(err)
		}
		seen[name] = true
	}
}

func addElement(v reflect.Value) reflect.Value {
	ln := v.Len() + 1
	if v.Cap() < ln {
		v.Grow(ln)
	}
	v.SetLen(ln)
	return v.Index(ln - 1)
}

func setVal(el reflect.Value, name, text string) error {
	for _, f := range reflect.VisibleFields(el.Type()) {
		if toSnakeCase(f.Name) == name {
			if !f.IsExported() {
				return fmt.Errorf("struct field %q must be exported", f.Name)
			}
			v := el.FieldByIndex(f.Index)
			switch v.Kind() {
			case reflect.String:
				v.SetString(text)
			case reflect.Int:
				i, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
				if err != nil {
					return fmt.Errorf(
						"invalid value for struct field %q: %v", f.Name, err)
				}
				v.SetInt(i)
			case reflect.Bool:
				v.SetBool(true)
			default:
				return fmt.Errorf("unexpected type %q of struct field %q",
					v.Kind(), f.Name)
			}
			return nil
		}
	}
	return fmt.Errorf("unexpected =%s=", name)
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToUpper(snake)
}

func (s *state) readDef() (string, error) {
	var line string
	for {
		// Skip empty lines and comments
		idx := bytes.IndexByte(s.rest, byte('\n'))
		if idx == -1 {
			line = string(s.rest)
		} else {
			line = string(s.rest[:idx])
		}
		line2 := strings.TrimSpace(line)
		if line2 == "" || line2[0] == '#' {
			if idx == -1 {
				s.rest = s.rest[len(s.rest):]
				// Found EOF.
				return "", nil
			} else {
				s.rest = s.rest[idx+1:]
				continue
			}
		} else {
			break
		}
	}
	name := s.checkDef(line)
	if name == "" {
		nr := s.currentLine()
		return "", fmt.Errorf("expecting token '=...=' at line %d of file %q: %s",
			nr, s.filename, line)
	}
	s.rest = s.rest[len(name)+2:]
	return name, nil
}

func (s *state) currentLine() int {
	return 1 + bytes.Count(s.src[0:len(s.src)-len(s.rest)], []byte("\n"))
}

func (s *state) checkDef(line string) string {
	if line == "" || line[0] != '=' {
		return ""
	}
	idx := strings.Index(line[1:], "=")
	if idx == -1 {
		return ""
	}
	name := line[1 : idx+1]
	if len(name) > 0 && isName(name) {
		return name
	}
	return ""
}

func (s *state) templDef() error {
	name, err := s.readTemplName()
	if err != nil {
		return err
	}
	text, err := s.readExpandedText()
	if err != nil {
		return err
	}
	text = strings.TrimSuffix(text, "\n")
	if len(text) == 0 {
		return fmt.Errorf("missing text after =TEMPL=%s", name)
	}
	fMap := template.FuncMap{
		// Get current date shifted by 'offset' days.
		"DATE": func(offset int) string {
			return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
		},
	}
	s.templates[name], err =
		template.New(name).Option("missingkey=zero").Funcs(fMap).Parse(text)
	return err
}

func (s *state) readTemplName() (string, error) {
	var line string
	idx := bytes.IndexByte(s.rest, byte('\n'))
	if idx == -1 {
		line = string(s.rest)
		s.rest = s.rest[len(s.rest):]
	} else {
		line = string(s.rest[:idx])
		s.rest = s.rest[idx:] // don't skip trailing newline
	}
	name := strings.TrimSpace(line)
	if len(name) == 0 {
		return "", errors.New("missing name after =TEMPL=")
	}
	if !isName(name) {
		return "", fmt.Errorf("invalid name after =TEMPL=: %q", name)
	}
	return name, nil
}

func isName(n string) bool {
	for _, ch := range n {
		if !(isLetter(ch) || isDecimal(ch)) {
			return false
		}
	}
	return true
}

func lower(ch rune) rune     { return ('a' - 'A') | ch }
func isDecimal(ch rune) bool { return '0' <= ch && ch <= '9' }

func isLetter(ch rune) bool {
	return 'a' <= lower(ch) && lower(ch) <= 'z' || ch == '_'
}

func (s *state) readExpandedText() (string, error) {
	line, err := s.doTemplSubst(s.readText())
	if err != nil {
		return "", err
	}
	return s.applySubst(line)
}

func (s *state) readText() string {
	// Check for single line
	line := s.getLine()
	s.rest = s.rest[len(line):]
	line = strings.TrimSpace(line)
	if line != "" {
		return line
	}
	// Read multiple lines up to start of next definition
	text := s.rest
	size := 0
	for {
		line := s.getLine()
		if name := s.checkDef(line); name != "" || line == "" {
			if name == "END" {
				s.rest = s.rest[len("=END="):]
			}
			return string(text[:size])
		}
		s.rest = s.rest[len(line):]
		size += len(line)
	}
}

// Match "]" in "]]]" as part of YAML sequence only after whitespace.
var templRx = regexp.MustCompile(`(?is)\[\[[a-z0-9_]+(\s+.*?\]?)?\]\]`)

// Substitute occurrences of [[name yaml-data]] by text of evaluated
// named template.
func (s *state) doTemplSubst(text string) (string, error) {
	var result strings.Builder
	// Position of text after previous occurrence of [[..]]
	prevIdx := 0

	il := templRx.FindAllStringIndex(text, -1)
	for _, p := range il {
		result.WriteString(text[prevIdx:p[0]])
		prevIdx = p[1]
		pair := text[p[0]+2 : p[1]-2] // without "[[" and "]]"
		var name string
		var data interface{}
		if i := strings.IndexAny(pair, " \t\n"); i != -1 {
			name = pair[:i]
			y := pair[i+1:]
			if err := yaml.Unmarshal([]byte(y), &data); err != nil {
				return "", fmt.Errorf(
					"invalid YAML data %q in call to template %q:\n %q", y, name, err)
			}
		} else {
			name = pair
		}
		t := s.templates[name]
		if t == nil {
			return "", fmt.Errorf("calling unknown template %q", name)
		}
		var b strings.Builder
		if err := t.Execute(&b, data); err != nil {
			return "", fmt.Errorf("%v", err)
		}
		result.WriteString(b.String())
	}
	result.WriteString(text[prevIdx:])
	return result.String(), nil
}

// Apply one or more substitutions to current textblock.
func (s *state) applySubst(text string) (string, error) {
	for {
		line := s.getLine()
		name := s.checkDef(line)
		if name != "SUBST" {
			break
		}
		s.rest = s.rest[len(line):]
		line = line[len("=SUBST="):]
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			return "", errors.New("invalid empty substitution")
		}
		parts := strings.Split(line[1:], line[0:1])
		if len(parts) != 3 || parts[2] != "" {
			return "", errors.New("invalid substitution: =SUBST=" + line)
		}
		text = strings.ReplaceAll(text, parts[0], parts[1])
	}
	return text, nil
}

func (s *state) getLine() string {
	idx := bytes.IndexByte(s.rest, byte('\n'))
	if idx == -1 {
		return string(s.rest)
	}
	return string(s.rest[:idx+1])
}

// PrepareFileOrDir creates directory dst and fills it with files
// described by input. Parts of input are marked by single lines of
// dashes followed by a filename.
// If no markers are given, no directory but a single file named dst
// is created.
// If input is string "NONE", input is assumed to be the empty string.
func PrepareFileOrDir(t *testing.T, dst, input string) {
	if input == "NONE" {
		input = ""
	}
	re := regexp.MustCompile(`(?ms)^-+[ ]*\S+[ ]*\n`)
	idxList := re.FindAllStringIndex(input, -1)

	write := func(file, data string) {
		dir := path.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Can't create directory for '%s': %v", file, err)
		}
		if err := os.WriteFile(file, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// No filename
	if idxList == nil {
		write(dst, input)
		return
	}
	if idxList[0][0] != 0 {
		t.Fatal("Missing file marker in first line")
	}
	for i, p := range idxList {
		marker := input[p[0] : p[1]-1] // without trailing "\n"
		pName := strings.Trim(marker, "- ")
		file := path.Join(dst, pName)
		start := p[1]
		end := len(input)
		if i+1 < len(idxList) {
			end = idxList[i+1][0]
		}
		data := input[start:end]
		write(file, data)
	}
	return
}

// Create inDir and fill it with files described by input.
// Parts of input are marked by single lines of dashes
// followed by a filename.
// If no markers are given, a file named single is created inside inDir.
// If single was created, it returns path of single, otherwise returns
// path of inDir.
func PrepareInDir(t *testing.T, inDir, single, input string) string {
	PrepareFileOrDir(t, inDir, input)
	if s, _ := os.Stat(inDir); s.IsDir() {
		return inDir
	}
	tmp := inDir + ".tmp"
	os.Rename(inDir, tmp)
	os.MkdirAll(inDir, 0755)
	file := filepath.Join(inDir, single)
	os.Rename(tmp, file)
	return file
}
