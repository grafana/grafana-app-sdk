package apiserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

type appAdmission struct {
	appGetter    func() app.App
	manifestData app.ManifestData
}

var _ admission.MutationInterface = (*appAdmission)(nil)
var _ admission.ValidationInterface = (*appAdmission)(nil)

func (ad *appAdmission) Admit(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	adApp := ad.appGetter()
	if adApp == nil {
		return admission.NewForbidden(a, errors.New("app is not initialized"))
	}

	adInfo := ad.getAdmissionInfo(a.GetKind())
	if adInfo == nil || !adInfo.SupportsAnyMutation() {
		return nil
	}

	req, err := translateAdmissionAttributes(a)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	supported := false
	for _, val := range adInfo.Mutation.Operations {
		if string(val) == string(req.Action) {
			supported = true
			break
		}
	}
	if !supported {
		return nil
	}

	res, err := adApp.Mutate(ctx, req)
	if err != nil {
		if errors.Is(err, app.ErrNotImplemented) {
			return nil
		}
		return admission.NewForbidden(a, err)
	}

	obj := a.GetObject()
	if obj != nil && res.UpdatedObject != nil {
		reflect.ValueOf(obj).Elem().Set(reflect.ValueOf(res.UpdatedObject).Elem())
	}
	return nil
}

func (ad *appAdmission) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	adApp := ad.appGetter()
	if adApp == nil {
		return admission.NewForbidden(a, errors.New("app is not initialized"))
	}

	adInfo := ad.getAdmissionInfo(a.GetKind())
	if adInfo == nil || !adInfo.SupportsAnyValidation() {
		return nil
	}

	req, err := translateAdmissionAttributes(a)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	supported := false
	for _, val := range adInfo.Validation.Operations {
		if string(val) == string(req.Action) {
			supported = true
			break
		}
	}
	if !supported {
		return nil
	}

	err = adApp.Validate(ctx, req)
	if err != nil {
		if errors.Is(err, app.ErrNotImplemented) {
			return nil
		}
		return admission.NewForbidden(a, err)
	}
	return nil
}

func (*appAdmission) Handles(_ admission.Operation) bool {
	return true
}

func (ad *appAdmission) getAdmissionInfo(gvk schema.GroupVersionKind) *app.AdmissionCapabilities {
	for _, v := range ad.manifestData.Versions {
		if gvk.Version != v.Name {
			continue
		}
		for _, k := range v.Kinds {
			if gvk.Kind != k.Kind {
				continue
			}
			return k.Admission
		}
	}
	return nil
}

func translateAdmissionAttributes(a admission.Attributes) (*app.AdmissionRequest, error) {
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

	userInfo := resource.AdmissionUserInfo{}
	// a.GetUserInfo() is nil for anonymous auth
	if a.GetUserInfo() != nil {
		extra := make(map[string]any)
		for k, v := range a.GetUserInfo().GetExtra() {
			extra[k] = any(v)
		}
		userInfo.UID = a.GetUserInfo().GetUID()
		userInfo.Username = a.GetUserInfo().GetName()
		userInfo.Groups = a.GetUserInfo().GetGroups()
		userInfo.Extra = extra
	}

	req := app.AdmissionRequest{
		Action:    action,
		Kind:      a.GetKind().Kind,
		Group:     a.GetKind().Group,
		Version:   a.GetKind().Version,
		UserInfo:  userInfo,
		Object:    obj,
		OldObject: oldObj,
	}

	return &req, nil
}
