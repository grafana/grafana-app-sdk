package v1alpha2

import "encoding/json"

type appManifestRole struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Kinds       []appManifesrRoleKind `json:"kinds"`
	Routes      []string              `json:"routes"`
}

type appManifesrRoleKind struct {
	Kind          string   `json:"kind"`
	PermissionSet *string  `json:"permissionSet,omitempty"`
	Verbs         []string `json:"verbs,omitempty"`
}

// UnmarshalJSON to unmarshal Kinds into the correct type
func (r *AppManifestRole) UnmarshalJSON(data []byte) error {
	m := appManifestRole{}
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	r.Title = m.Title
	r.Description = m.Description
	r.Routes = make([]string, len(m.Routes))
	copy(r.Routes, m.Routes)
	r.Kinds = make([]AppManifestRoleKind, len(m.Kinds))
	for idx, k := range m.Kinds {
		if k.PermissionSet != nil {
			r.Kinds[idx] = AppManifestRoleKindWithPermissionSet{
				Kind:          k.Kind,
				PermissionSet: AppManifestRoleKindWithPermissionSetPermissionSet(*k.PermissionSet),
			}
		} else {
			verbs := make([]string, len(k.Verbs))
			copy(verbs, k.Verbs)
			r.Kinds[idx] = AppManifestRoleKindWithVerbs{
				Kind:  k.Kind,
				Verbs: verbs,
			}
		}
	}
	return nil
}
