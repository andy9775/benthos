// Copyright (c) 2018 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, sub to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package text

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Jeffail/benthos/lib/message"
)

func TestFunctionVarDetection(t *testing.T) {
	tests := map[string]bool{
		"foo ${!foo_bar} baz":                       true,
		"foo ${!foo_bar} baz ${!foo_baz}":           true,
		"foo $!foo} baz $!but_not_this}":            false,
		"foo ${!baz ${!or_this":                     false,
		"foo ${baz} ${or_this}":                     false,
		"nothing $ here boss {!}":                   false,
		"foo ${!foo_bar:arg1} baz":                  true,
		"foo ${!foo_bar:} baz":                      false,
		"foo ${!foo_bar:arg1} baz ${!foo_baz:arg2}": true,
		"foo $!foo:arg2} baz $!but_not_this:}":      false,
		"nothing $ here boss {!:argnope}":           false,
	}

	for in, exp := range tests {
		act := ContainsFunctionVariables([]byte(in))
		if act != exp {
			t.Errorf("Wrong result for '%v': %v != %v", in, act, exp)
		}
	}
}

func TestMetadataFunction(t *testing.T) {
	msg := message.New(nil)
	msg.SetMetadata("foo", "bar")
	msg.SetMetadata("baz", "qux")

	act := string(ReplaceFunctionVariables(
		msg, []byte(`foo ${!metadata:foo} baz`),
	))
	if exp := "foo bar baz"; act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	act = string(ReplaceFunctionVariables(
		msg, []byte(`foo ${!metadata:bar} baz`),
	))
	if exp := "foo  baz"; act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	act = string(ReplaceFunctionVariables(
		msg, []byte(`${!metadata}`),
	))
	if exp := `{"baz":"qux","foo":"bar"}`; act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}
}

func TestJSONFunction(t *testing.T) {
	type testCase struct {
		name   string
		input  []string
		arg    string
		result string
	}

	tests := []testCase{
		{
			name: "json func 1",
			input: []string{
				`{"foo":{"bar":"baz"}}`,
			},
			arg:    "foo ${!json_field:foo.bar,0} baz",
			result: "foo baz baz",
		},
		{
			name: "json func 2",
			input: []string{
				`{"foo":{"bar":"baz"}}`,
			},
			arg:    "foo ${!json_field:foo.bar,1} baz",
			result: "foo null baz",
		},
		{
			name: "json func 3",
			input: []string{
				`{"foo":{"bar":"baz"}}`,
			},
			arg:    "foo ${!json_field:foo.baz,0} baz",
			result: "foo null baz",
		},
		{
			name: "json func 4",
			input: []string{
				`{"foo":{"bar":{"baz":1}}}`,
			},
			arg:    "foo ${!json_field:foo.bar,0} baz",
			result: `foo {"baz":1} baz`,
		},
		{
			name: "json func 5",
			input: []string{
				`{"foo":{"bar":{"baz":1}}}`,
			},
			arg:    "foo ${!json_field:foo.bar,0} baz",
			result: `foo {"baz":1} baz`,
		},
		{
			name: "json func 6",
			input: []string{
				`{"foo":{"bar":5}}`,
			},
			arg:    "foo ${!json_field:foo.bar} baz",
			result: `foo 5 baz`,
		},
		{
			name: "json func 7",
			input: []string{
				`{"foo":{"bar":false}}`,
			},
			arg:    "foo ${!json_field:foo.bar} baz",
			result: `foo false baz`,
		},
	}

	for _, test := range tests {
		exp := test.result
		parts := [][]byte{}
		for _, input := range test.input {
			parts = append(parts, []byte(input))
		}
		act := string(ReplaceFunctionVariables(
			message.New(parts),
			[]byte(test.arg),
		))
		if act != exp {
			t.Errorf("Wrong result for test '%v': %v != %v", test.name, act, exp)
		}
	}
}

func TestFunctionSwapping(t *testing.T) {
	hostname, _ := os.Hostname()

	exp := fmt.Sprintf("foo %v baz", hostname)
	act := string(ReplaceFunctionVariables(nil, []byte("foo ${!hostname} baz")))
	if act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	exp = "foo ${!} baz"
	act = string(ReplaceFunctionVariables(nil, []byte("foo ${!} baz")))
	if act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	exp = "foo ${!does_not_exist} baz"
	act = string(ReplaceFunctionVariables(nil, []byte("foo ${!does_not_exist} baz")))
	if act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	now := time.Now()
	tStamp := string(ReplaceFunctionVariables(nil, []byte("${!timestamp_unix_nano}")))

	nanoseconds, err := strconv.ParseInt(tStamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	tThen := time.Unix(0, nanoseconds)

	if tThen.Sub(now).Seconds() > 5.0 {
		t.Errorf("Timestamps too far out of sync: %v and %v", tThen, now)
	}

	now = time.Now()
	tStamp = string(ReplaceFunctionVariables(nil, []byte("${!timestamp_unix}")))

	seconds, err := strconv.ParseInt(tStamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	tThen = time.Unix(seconds, 0)

	if tThen.Sub(now).Seconds() > 5.0 {
		t.Errorf("Timestamps too far out of sync: %v and %v", tThen, now)
	}

	now = time.Now()
	tStamp = string(ReplaceFunctionVariables(nil, []byte("${!timestamp_unix:10}")))

	var secondsF float64
	secondsF, err = strconv.ParseFloat(tStamp, 64)
	if err != nil {
		t.Fatal(err)
	}
	tThen = time.Unix(int64(secondsF), 0)

	if tThen.Sub(now).Seconds() > 5.0 {
		t.Errorf("Timestamps too far out of sync: %v and %v", tThen, now)
	}

	now = time.Now()
	tStamp = string(ReplaceFunctionVariables(nil, []byte("${!timestamp}")))

	tThen, err = time.Parse("Mon Jan 2 15:04:05 -0700 MST 2006", tStamp)
	if err != nil {
		t.Fatal(err)
	}

	if tThen.Sub(now).Seconds() > 5.0 {
		t.Errorf("Timestamps too far out of sync: %v and %v", tThen, now)
	}
}

func TestEchoFunction(t *testing.T) {
	tests := map[string]string{
		"foo ${!echo:bar}":              "foo bar",
		"foo ${!echo}":                  "foo ",
		"foo ${!echo:bar} ${!echo:baz}": "foo bar baz",
	}

	for input, exp := range tests {
		act := string(ReplaceFunctionVariables(nil, []byte(input)))
		if exp != act {
			t.Errorf("Wrong results for input (%v): %v != %v", input, act, exp)
		}
	}
}

func TestCountersFunction(t *testing.T) {
	tests := [][2]string{
		{"foo1: ${!count:foo}", "foo1: 1"},
		{"bar1: ${!count:bar}", "bar1: 1"},
		{"foo2: ${!count:foo} ${!count:foo}", "foo2: 2 3"},
		{"bar2: ${!count:bar} ${!count:bar}", "bar2: 2 3"},
		{"foo3: ${!count:foo} ${!count:foo}", "foo3: 4 5"},
		{"bar3: ${!count:bar} ${!count:bar}", "bar3: 4 5"},
	}

	for _, test := range tests {
		input := test[0]
		exp := test[1]
		act := string(ReplaceFunctionVariables(nil, []byte(input)))
		if exp != act {
			t.Errorf("Wrong results for input (%v): %v != %v", input, act, exp)
		}
	}
}
