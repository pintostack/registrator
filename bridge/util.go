package bridge

import (
	"log"
	"strconv"
	"strings"

	"github.com/cenkalti/backoff"
	dockerapi "github.com/fsouza/go-dockerclient"
)

func retry(fn func() error) error {
	return backoff.Retry(fn, backoff.NewExponentialBackOff())
}

func mapDefault(m map[string]string, key, default_ string) string {
	v, ok := m[key]
	if !ok || v == "" {
		return default_
	}
	return v
}

func combineTags(tagParts ...string) []string {
	tags := make([]string, 0)
	for _, element := range tagParts {
		if element != "" {
			tags = append(tags, strings.Split(element, ",")...)
		}
	}
	return tags
}

// find "PORTX=port" entry and return X
// "PORT_Y=port" are ignored
// if X is empty then return "0"
func findPortIndex(env []string, port string) string {
	// TODO: use regexp!
	for _, kv := range env {
		kvp := strings.SplitN(kv, "=", 2)
		if len(kvp) < 2 {
			continue
		}
		k, v := kvp[0], kvp[1]
		if v != port {
			continue
		}
		if strings.HasPrefix(k, "PORT") {
			idx := k[4:] // remove "PORT"
			_, err := strconv.Atoi(idx)
			if err == nil {
				return idx
			}
		}
	}

	return "" // not found or disabled
}

func expandString(s string, vals map[string]string) string {
	for k, v := range vals {
		s = strings.Replace(s, k, v, -1)
	}
	return s
}

// portIndex is empty if marathon ports are disabled
func serviceMetaData(config *dockerapi.Config, port string, portIndex string) (map[string]string, map[string]bool) {
	meta := config.Env
	log.Printf("environment: %q", config.Env)
	log.Printf("labels: %v", config.Labels)
	for k, v := range config.Labels {
		meta = append(meta, k+"="+v)
	}
	metadata := make(map[string]string)
	metadataFromPort := make(map[string]bool)
	for _, kv := range meta {
		kvp := strings.SplitN(kv, "=", 2)
		if strings.HasPrefix(kvp[0], "SERVICE_") && len(kvp) > 1 {
			//log.Printf("%q has service prefix, inspecting...", kv)
			k, v := strings.TrimPrefix(kvp[0], "SERVICE_"), kvp[1]
			key := strings.ToLower(k)
			if metadataFromPort[key] {
				log.Printf("%q already set by port", key)
				continue
			}

			// check for SERVICE_XXXX_key=value
			// where XXXXX is a container port
			portkey := strings.SplitN(k, "_", 2)
			_, err := strconv.Atoi(portkey[0])
			if err == nil && len(portkey) > 1 {
				if portkey[0] != port {
					continue
				}

				key = strings.ToLower(portkey[1])
				metadata[key] = v
				metadataFromPort[key] = true
				continue
			}

			// check for SERVICE_PORTX_key=value
			if len(portIndex) > 0 && len(portkey) > 1 {
				n := "PORT" + portIndex
				if portkey[0] != n {
					continue
				}

				key = strings.ToLower(portkey[1])
				metadata[key] = v
				metadataFromPort[key] = true
				continue
			}

			// otherwise SERVICE_key=value
			metadata[key] = v
		}
	}
	return metadata, metadataFromPort
}

func servicePort(container *dockerapi.Container, port dockerapi.Port, published []dockerapi.PortBinding) ServicePort {
	var hp, hip, ep, ept, eip string
	if len(published) > 0 {
		hp = published[0].HostPort
		hip = published[0].HostIP
	}
	if hip == "" {
		hip = "0.0.0.0"
	}
	exposedPort := strings.Split(string(port), "/")
	ep = exposedPort[0]
	if len(exposedPort) == 2 {
		ept = exposedPort[1]
	} else {
		ept = "tcp" // default
	}

	// Nir: support docker NetworkSettings
	eip = container.NetworkSettings.IPAddress
	if eip == "" {
		for _, network := range container.NetworkSettings.Networks {
			eip = network.IPAddress
		}
	}

	return ServicePort{
		HostPort:          hp,
		HostIP:            hip,
		ExposedPort:       ep,
		ExposedIP:         eip,
		PortType:          ept,
		ContainerID:       container.ID,
		ContainerHostname: container.Config.Hostname,
		container:         container,
	}
}
