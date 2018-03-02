// Copyright (c) 2018 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	_ "net/http/pprof"
	"path/filepath"

	"github.com/Jeffail/benthos/lib/api"
	"github.com/Jeffail/benthos/lib/input"
	"github.com/Jeffail/benthos/lib/output"
	yaml "gopkg.in/yaml.v2"
)

//------------------------------------------------------------------------------

// Config is the benthos configuration struct.
type Config struct {
	HTTP   api.Config    `json:"http" yaml:"http"`
	Input  input.Config  `json:"input" yaml:"input"`
	Output output.Config `json:"output" yaml:"output"`
}

// NewConfig returns a new configuration with default values.
func NewConfig() Config {
	return Config{
		HTTP:   api.NewConfig(),
		Input:  input.NewConfig(),
		Output: output.NewConfig(),
	}
}

// Sanitised returns a sanitised copy of the Benthos configuration, meaning
// fields of no consequence (unused inputs, outputs, processors etc) are
// excluded.
func (c Config) Sanitised() (interface{}, error) {
	inConf, err := input.SanitiseConfig(c.Input)
	if err != nil {
		return nil, err
	}

	var outConf interface{}
	outConf, err = output.SanitiseConfig(c.Output)
	if err != nil {
		return nil, err
	}

	return struct {
		HTTP   interface{} `json:"http" yaml:"http"`
		Input  interface{} `json:"input" yaml:"input"`
		Output interface{} `json:"output" yaml:"output"`
	}{
		HTTP:   c.HTTP,
		Input:  inConf,
		Output: outConf,
	}, nil
}

//------------------------------------------------------------------------------

func main() {
	configsDir := "./config"
	flag.StringVar(&configsDir, "dir", configsDir, "The directory to write config examples")
	flag.Parse()

	// Get list of all types (both input and output).
	typeMap := map[string]struct{}{}
	for t := range input.Constructors {
		typeMap[t] = struct{}{}
	}
	for t := range output.Constructors {
		typeMap[t] = struct{}{}
	}

	// Generate configs for all types.
	for t := range typeMap {
		conf := NewConfig()
		conf.Input.Processors = nil
		conf.Output.Processors = nil

		if _, exists := input.Constructors[t]; exists {
			conf.Input.Type = t
		}
		if _, exists := output.Constructors[t]; exists {
			conf.Output.Type = t
		}

		sanit, err := conf.Sanitised()
		if err != nil {
			panic(err)
		}

		resBytes := []byte("# This file was auto generated by benthos_config_gen.\n")

		var cBytes []byte
		cBytes, err = yaml.Marshal(sanit)
		if err != nil {
			panic(err)
		}
		resBytes = append(resBytes, cBytes...)

		confPath := filepath.Join(configsDir, t+".yaml")
		if err = ioutil.WriteFile(confPath, resBytes, 0644); err != nil {
			panic(err)
		}
		fmt.Printf("Generated '%v' config at: %v\n", t, confPath)
	}
}

//------------------------------------------------------------------------------
