package kindsys_test

import (
	"testing"

	"github.com/grafana/grafana-app-sdk/kindsys"
)

func TestFramework(t *testing.T) {
	// please don't panic, that's all I ask
	_ = kindsys.CUEFramework(nil)
}
