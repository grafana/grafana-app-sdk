package apiserver

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apiserver/pkg/admission"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

type appAdmission struct {
	app app.App
}

var _ admission.MutationInterface = (*appAdmission)(nil)
var _ admission.ValidationInterface = (*appAdmission)(nil)

func (ad *appAdmission) Admit(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	req, err := translateAdmissionAttributes(a)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	res, err := ad.app.Mutate(ctx, req)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	obj := a.GetObject()
	if obj != nil && res.UpdatedObject != nil {
		reflect.ValueOf(obj).Elem().Set(reflect.ValueOf(res.UpdatedObject).Elem())
	}
	return nil
}

func (ad *appAdmission) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	req, err := translateAdmissionAttributes(a)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	err = ad.app.Validate(ctx, req)
	if err != nil {
		return admission.NewForbidden(a, err)
	}
	return nil
}

func (*appAdmission) Handles(_ admission.Operation) bool {
	return true
}

func translateAdmissionAttributes(a admission.Attributes) (*app.AdmissionRequest, error) {
	extra := make(map[string]any)
	for k, v := range a.GetUserInfo().GetExtra() {
		extra[k] = any(v)
	}

	var action resource.AdmissionAction
	switch a.GetOperation() {
	case admission.Create:
		action = resource.AdmissionActionCreate
	case admission.Update:
		action = resource.AdmissionActionUpdate
	case admission.Delete:
		action = resource.AdmissionActionDelete
	case admission.Connect:
		action = resource.AdmissionActionConnect
	default:
		return nil, fmt.Errorf("unknown admission operation: %v", a.GetOperation())
	}

	var (
		obj    resource.Object
		oldObj resource.Object
		ok     bool
	)

	if a.GetObject() != nil {
		obj, ok = a.GetObject().(resource.Object)
		if !ok {
			return nil, admission.NewForbidden(a, fmt.Errorf("object is not a resource.Object"))
		}
	}

	if a.GetOldObject() != nil {
		oldObj, ok = a.GetOldObject().(resource.Object)
		if !ok {
			return nil, admission.NewForbidden(a, fmt.Errorf("oldObject is not a resource.Object"))
		}
	}

	req := app.AdmissionRequest{
		Action:  action,
		Kind:    a.GetKind().Kind,
		Group:   a.GetKind().Group,
		Version: a.GetKind().Version,
		UserInfo: resource.AdmissionUserInfo{
			UID:      a.GetUserInfo().GetUID(),
			Username: a.GetUserInfo().GetName(),
			Groups:   a.GetUserInfo().GetGroups(),
			Extra:    extra,
		},
		Object:    obj,
		OldObject: oldObj,
	}

	return &req, nil
}
