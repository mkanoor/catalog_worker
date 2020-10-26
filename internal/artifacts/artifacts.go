package artifacts

import (
	"encoding/json"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Sanctify the JSON payload for artifacts. The attribute key in the artifacts
// map should start with expose_to_cloud_redhat_com_ else they are excluded
func Sanctify(data map[string]interface{}) (map[string]interface{}, error) {
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
