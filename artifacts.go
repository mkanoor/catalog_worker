package main

import (
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"strings"
)

func sanctifyArtifacts(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range data {
		if strings.HasPrefix(k, "expose_to_cloud_redhat_com_") {
			result[k] = v
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		log.Println("Error marshaling to json error:", err)
		return nil, err
	}

	if len(b) > 1024 {
		err = errors.New("Artifacts is greater than 1024 bytes")
		return nil, err
	}
	return result, nil
}
