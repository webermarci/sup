package control

import "github.com/webermarci/sup"

type ExposedSignal struct {
	ID    string   `json:"id"`
	Spec  sup.Spec `json:"spec"`
	Type  string   `json:"type"`
	Value any      `json:"value"`
}
