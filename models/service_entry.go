package models

import (
	"encoding/json"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kiali/kiali/kubernetes"
)

type ServiceEntries []ServiceEntry
type ServiceEntry struct {
	meta_v1.TypeMeta
	Metadata meta_v1.ObjectMeta `json:"metadata"`
	Spec     struct {
		Hosts            interface{}               `json:"hosts"`
		Addresses        interface{}               `json:"addresses"`
		Ports            interface{}               `json:"ports"`
		Location         interface{}               `json:"location"`
		Resolution       interface{}               `json:"resolution"`
		Endpoints        []ServiceEntriesEndpoints `json:"endpoints"`
		WorkloadSelector interface{}               `json:"workloadSelector"`
		ExportTo         interface{}               `json:"exportTo"`
		SubjectAltNames  interface{}               `json:"subjectAltNames"`
	} `json:"spec"`
}

func (ses *ServiceEntries) Parse(serviceEntries []kubernetes.IstioObject) {
	for _, se := range serviceEntries {
		serviceEntry := ServiceEntry{}
		serviceEntry.Parse(se)
		*ses = append(*ses, serviceEntry)
	}
}

func (se *ServiceEntry) Parse(serviceEntry kubernetes.IstioObject) {
	epp := &[]ServiceEntriesEndpoints{}
	ep, _ := json.Marshal(serviceEntry.GetSpec()["endpoints"])
	json.Unmarshal(ep, epp)
	se.TypeMeta = serviceEntry.GetTypeMeta()
	se.Metadata = serviceEntry.GetObjectMeta()
	se.Spec.Hosts = serviceEntry.GetSpec()["hosts"]
	se.Spec.Addresses = serviceEntry.GetSpec()["addresses"]
	se.Spec.Ports = serviceEntry.GetSpec()["ports"]
	se.Spec.Location = serviceEntry.GetSpec()["location"]
	se.Spec.Resolution = serviceEntry.GetSpec()["resolution"]
	se.Spec.Endpoints = *epp
	se.Spec.WorkloadSelector = serviceEntry.GetSpec()["serviceEntry"]
	se.Spec.ExportTo = serviceEntry.GetSpec()["exportTo"]
	se.Spec.SubjectAltNames = serviceEntry.GetSpec()["subjectAltNames"]
}

type ServiceEntriesEndpoints struct {
	Address string
}
