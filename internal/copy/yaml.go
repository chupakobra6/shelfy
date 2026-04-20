package copy

import "gopkg.in/yaml.v3"

func yamlUnmarshal(body []byte, dest any) error {
	return yaml.Unmarshal(body, dest)
}
