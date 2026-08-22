package apiserver

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metatable "k8s.io/apimachinery/pkg/api/meta/table"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

var swaggerMetadataDescriptions = metav1.ObjectMeta{}.SwaggerDoc()

type columnDefinition struct {
	valueFunc func(resource.Object) (any, error)
	header    metav1.TableColumnDefinition
}

type additionalColumnsTableConvertor struct {
	headers []metav1.TableColumnDefinition
	columns []columnDefinition
}

// newTableConvertor creates a rest.TableConvertor from Schema-provided TableColumns.
func newTableConvertor(columns []resource.TableColumn) rest.TableConvertor {
	c := &additionalColumnsTableConvertor{
		headers: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name", Description: swaggerMetadataDescriptions["name"]},
		},
	}

	for _, col := range columns {
		desc := fmt.Sprintf("Custom resource definition column (in JSONPath format): %s", col.JSONPath)
		if col.Description != "" {
			desc = col.Description
		}

		c.columns = append(c.columns, columnDefinition{
			valueFunc: col.ValueFunc,
			header: metav1.TableColumnDefinition{
				Name:        col.Name,
				Type:        col.Type,
				Format:      col.Format,
				Description: desc,
				Priority:    col.Priority,
			},
		})
		c.headers = append(c.headers, c.columns[len(c.columns)-1].header)
	}

	return c
}

func (c *additionalColumnsTableConvertor) ConvertToTable(_ context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{}
	opt, ok := tableOptions.(*metav1.TableOptions)
	noHeaders := ok && opt != nil && opt.NoHeaders
	if !noHeaders {
		table.ColumnDefinitions = c.headers
	}

	if m, err := meta.ListAccessor(obj); err == nil {
		table.ResourceVersion = m.GetResourceVersion()
		table.Continue = m.GetContinue()
		table.RemainingItemCount = m.GetRemainingItemCount()
	} else {
		if m, err := meta.CommonAccessor(obj); err == nil {
			table.ResourceVersion = m.GetResourceVersion()
		}
	}

	var tableErr error
	table.Rows, tableErr = metatable.MetaToTableRow(obj, func(obj runtime.Object, _ metav1.Object, name, _ string) ([]any, error) {
		cells := make([]any, 1, 1+len(c.columns))
		cells[0] = name
		resourceObj, ok := obj.(resource.Object)
		if !ok {
			for range c.columns {
				cells = append(cells, nil)
			}
			return cells, nil
		}
		for _, col := range c.columns {
			value, err := col.valueFunc(resourceObj)
			if err != nil || value == nil {
				cells = append(cells, nil)
				continue
			}
			cells = append(cells, value)
		}
		return cells, nil
	})
	return table, tableErr
}
