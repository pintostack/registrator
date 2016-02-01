package mesosconsul

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

var (
	TAG = "mesos-consul"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), TAG)
}

func (r *MesosConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatalf("%s: %s", TAG, uri.Scheme)
	}
	log.Printf("%s: creating new adapter, uri: %s, config:%#v", TAG, uri, config)
	// TODO: get mesos url from configuration, from zookeeper? from uri's query?
	mesos := NewMesos("http://master-1.node.consul:5050")

	host, err := os.Hostname()
	if err != nil {
		log.Fatalf("%s: failed to get hostname: %s", TAG, err)
	}

	return &MesosConsulAdapter{
		consul: client,
		mesos:  mesos,
		host:   host,
	}
}

type MesosConsulAdapter struct {
	consul *consulapi.Client
	mesos  *Mesos
	host   string
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *MesosConsulAdapter) Ping() error {
	status := r.consul.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}

	log.Printf("%s: current leader %s", TAG, leader)
	return nil // OK
}

// Register registers service in local consul agent.
// Somу service information is updated based on mesos state.
func (r *MesosConsulAdapter) Register(service *bridge.Service) error {
	log.Printf("%s: registering service: %#v", TAG, service)

	if err := r.mesos.Refresh(); err != nil {
		log.Printf("%s: failed to update mesos state: %s", TAG, err)
		return fmt.Errorf("failed to update mesos state: %s", err)
	}

	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)

	// TODO: update name, port based on mesos information!
	// tags, labels etc

	return r.consul.Agent().ServiceRegister(registration)
}

func (r *MesosConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Script = r.interpolateService(script, service)
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else {
		return nil
	}
	if check.Script != "" || check.HTTP != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	return check
}

func (r *MesosConsulAdapter) Deregister(service *bridge.Service) error {
	log.Printf("%s: deregistering service: %#v", TAG, service)
	return r.consul.Agent().ServiceDeregister(service.ID)
}

func (r *MesosConsulAdapter) Refresh(service *bridge.Service) error {
	return nil
}

func (r *MesosConsulAdapter) Services() ([]*bridge.Service, error) {
	services, err := r.consul.Agent().Services()
	if err != nil {
		return []*bridge.Service{}, err
	}
	out := make([]*bridge.Service, len(services))
	i := 0
	for _, v := range services {
		s := &bridge.Service{
			ID:   v.ID,
			Name: v.Service,
			Port: v.Port,
			Tags: v.Tags,
			IP:   v.Address,
		}
		out[i] = s
		i++
	}
	return out, nil
}
