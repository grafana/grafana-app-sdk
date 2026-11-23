package jennies

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRouteName(t *testing.T) {
	tests := []struct {
		name   string
		method string
		route  string
		expect string
	}{
		{
			name:   "simple path",
			method: http.MethodGet,
			route:  "alerts",
			expect: "getAlerts",
		},
		{
			name:   "multi-segment path capitalizes each segment",
			method: http.MethodPost,
			route:  "alerts/urgent",
			expect: "createAlertsUrgent",
		},
		{
			name:   "three-segment path",
			method: http.MethodGet,
			route:  "alerts/urgent/summary",
			expect: "getAlertsUrgentSummary",
		},
		{
			name:   "leading slash is stripped",
			method: http.MethodGet,
			route:  "/alerts/urgent",
			expect: "getAlertsUrgent",
		},
		{
			name:   "delete method",
			method: http.MethodDelete,
			route:  "alerts/urgent",
			expect: "deleteAlertsUrgent",
		},
		{
			name:   "put method",
			method: http.MethodPut,
			route:  "alerts/urgent",
			expect: "replaceAlertsUrgent",
		},
		{
			name:   "special characters removed from segment",
			method: http.MethodGet,
			route:  "alerts-v2/urgent",
			expect: "getAlertsv2Urgent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultRouteName(tt.method, tt.route)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestToExportedFieldName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "already exported",
			input:  "Alerts",
			expect: "Alerts",
		},
		{
			name:   "lowercase first letter",
			input:  "alerts",
			expect: "Alerts",
		},
		{
			name:   "single character",
			input:  "a",
			expect: "A",
		},
		{
			name:   "special characters removed",
			input:  "alerts/urgent",
			expect: "Alertsurgent",
		},
		{
			name:   "hyphens removed",
			input:  "alerts-v2",
			expect: "Alertsv2",
		},
		{
			name:   "underscores preserved",
			input:  "alerts_v2",
			expect: "Alerts_v2",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toExportedFieldName(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}
