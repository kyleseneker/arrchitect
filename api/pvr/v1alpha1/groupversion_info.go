// Package v1alpha1 contains the arrchitect.dev/v1alpha1 kinds for Sonarr, Radarr, Lidarr and Readarr.
// +kubebuilder:object:generate=true
// +groupName=arrchitect.dev
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "arrchitect.dev", Version: "v1alpha1"}
	GroupVersion       = SchemeGroupVersion
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Sonarr{}, &SonarrList{},
		&SonarrIndexer{}, &SonarrIndexerList{},
		&SonarrDownloadClient{}, &SonarrDownloadClientList{},
		&Radarr{}, &RadarrList{},
		&RadarrIndexer{}, &RadarrIndexerList{},
		&RadarrDownloadClient{}, &RadarrDownloadClientList{},
		&Lidarr{}, &LidarrList{},
		&LidarrIndexer{}, &LidarrIndexerList{},
		&LidarrDownloadClient{}, &LidarrDownloadClientList{},
		&Readarr{}, &ReadarrList{},
		&ReadarrIndexer{}, &ReadarrIndexerList{},
		&ReadarrDownloadClient{}, &ReadarrDownloadClientList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
