package core

type IRegion interface {
	GetHosts() []string

	GetAuthRegion() string
}
