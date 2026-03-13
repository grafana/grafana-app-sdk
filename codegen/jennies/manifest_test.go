package jennies

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinKindNames(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect string
	}{
		{
			name:   "empty",
			input:  []string{},
			expect: "",
		},
		{
			name:   "single kind",
			input:  []string{"Blueprints"},
			expect: "Blueprints",
		},
		{
			name:   "two kinds",
			input:  []string{"Blueprints", "Timelines"},
			expect: "Blueprints and Timelines",
		},
		{
			name:   "three kinds with Oxford comma",
			input:  []string{"Blueprints", "StepTypes", "Timelines"},
			expect: "Blueprints, StepTypes, and Timelines",
		},
		{
			name:   "four kinds",
			input:  []string{"As", "Bs", "Cs", "Ds"},
			expect: "As, Bs, Cs, and Ds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinKindNames(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}
