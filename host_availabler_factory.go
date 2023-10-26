package core

type HostAvailablerFactory interface {
	NewHostAvailabler(projectID string, hosts []string, mainHost string, skipFetchHosts bool) (HostAvailabler, error)
}

type HostAvailablerFactoryBase struct {
}

func (h *HostAvailablerFactoryBase) NewHostAvailabler(projectID string, hosts []string, mainHost string, skipFetchHosts bool) (HostAvailabler, error) {
	return NewPingHostAvailabler(hosts, projectID, nil, mainHost, skipFetchHosts)
}
