// +kubebuilder:object:generate=true
// +groupName=finops.krateo.io
// +versionName=v1
package v1

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "finops.krateo.io", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme

	Kind             = reflect.TypeOf(Notebook{}).Name()
	GroupKind        = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}.String()
	KindAPIVersion   = Kind + "." + GroupVersion.String()
	GroupVersionKind = GroupVersion.WithKind(Kind)
)
