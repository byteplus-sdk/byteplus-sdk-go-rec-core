package core

type HostAvailablerFactory interface {
	NewHostAvailabler(projectID string, hosts []string) (HostAvailabler, error)
}

type HostAvailablerFactoryBase struct {
}

func (h *HostAvailablerFactoryBase) NewHostAvailabler(projectID string, hosts []string) (HostAvailabler, error) {
	return NewPingHostAvailabler(hosts, projectID, nil)
}
