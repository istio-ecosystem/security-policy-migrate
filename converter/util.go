package converter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectStruct struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              map[string]interface{} `json:"spec,omitempty"`
}
