package apiserver

import (
	"context"
	"fmt"
	"reflect"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apiserver/pkg/admission"
)

type appAdmission struct {
	app app.App
}

var _ admission.MutationInterface = (*appAdmission)(nil)
var _ admission.ValidationInterface = (*appAdmission)(nil)

func (ad *appAdmission) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
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

func (ad *appAdmission) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
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

func (a *appAdmission) Handles(operation admission.Operation) bool {
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
	)

	if a.GetObject() != nil {
		obj = a.GetObject().(resource.Object)
	}

	if a.GetOldObject() != nil {
		oldObj = a.GetOldObject().(resource.Object)
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
