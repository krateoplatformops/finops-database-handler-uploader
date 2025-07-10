// +kubebuilder:object:generate=true
package v1

import (
	finopstypes "github.com/krateoplatformops/finops-data-types/api/v1"
	prv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type Notebook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotebookSpec   `json:"spec,omitempty"`
	Status NotebookStatus `json:"status,omitempty"`
}

type NotebookSpec struct {
	// +kubebuilder:validation:Pattern=^([iI][nN][lL][iI][nN][eE]|[aA][pP][iI])$
	Type string `yaml:"type" json:"type"`
	// +optional
	Inline string `yaml:"inline" json:"inline"`
	// +optional
	API finopstypes.API `yaml:"api" json:"api"`
}

type NotebookStatus struct {
	prv1.ConditionedStatus `json:",inline"`
}

//+kubebuilder:object:root=true

type NotebookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Notebook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Notebook{}, &NotebookList{})
}

func (mg *Notebook) GetCondition(ct prv1.ConditionType) prv1.Condition {
	return mg.Status.GetCondition(ct)
}

func (mg *Notebook) SetConditions(c ...prv1.Condition) {
	mg.Status.SetConditions(c...)
}
