package main

import (
	"encoding/json"
	"io/ioutil"
)

// RootConfiguration is the json configuration file
type RootConfiguration struct {
	Services []ServiceConfiguration
}

// ServiceConfiguration is the definition of one service
type ServiceConfiguration struct {
	UserID           uint32
	GroupID          uint32
	WorkingDirectory string
	Command          string
}

func getRootConfiguration(path string) (*RootConfiguration, error) {
	jsonBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rootConfiguration RootConfiguration
	err = json.Unmarshal(jsonBytes, &rootConfiguration)

	return &rootConfiguration, err
}
