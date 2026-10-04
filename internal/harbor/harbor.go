// Package harbor imports a documented subset. Unmapped fields stay visible.
package harbor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

var known = map[string]struct{}{
	"schema_version": {},
	"name":           {},
	"instruction":    {},
}

type Result struct {
	Name              string   `json:"name"`
	Prompt            string   `json:"prompt"`
	UnsupportedFields []string `json:"unsupported_fields"`
	AutoMerge         bool     `json:"auto_merge"`
	IsolationAssumed  string   `json:"isolation_assumed"`
}

// Import reads a Harbor subset document. It does not create a run or merge code.
func Import(raw []byte) (Result, error) {
	var node map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&node); err != nil {
		var y map[string]any
		if yerr := yaml.Unmarshal(raw, &y); yerr != nil {
			return Result{}, fmt.Errorf("harbor subset is not json or yaml")
		}
		node = y
	}
	name, _ := node["name"].(string)
	prompt, _ := node["instruction"].(string)
	if name == "" || prompt == "" {
		return Result{}, fmt.Errorf("harbor subset needs name and instruction")
	}
	var unsupported []string
	for key := range node {
		if _, ok := known[key]; !ok {
			unsupported = append(unsupported, key)
		}
	}
	sort.Strings(unsupported)
	return Result{
		Name: name, Prompt: prompt, UnsupportedFields: unsupported,
		AutoMerge: false, IsolationAssumed: "未声明。Harbor 的环境隔离不会被自动当成 agentlab 的执行器。",
	}, nil
}
