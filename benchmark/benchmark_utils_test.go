package benchmark_test

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientfeatures "k8s.io/client-go/features"
	"k8s.io/klog/v2"
)

var loggerSuppressed bool

func suppressLogger() error {
	if loggerSuppressed {
		return nil
	}

	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "false")
	_ = flag.Set("v", "0")
	klog.SetOutput(io.Discard)

	loggerSuppressed = true
	return logging.InitializerDefaultLogger(io.Discard, logging.Options{
		Format: logging.FormatText,
		Level:  slog.LevelError,
	})
}

// generateTestObjects creates N UntypedObjects with realistic metadata.
func generateTestObjects(count int) []resource.Object {
	objects := make([]resource.Object, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		obj := &resource.UntypedObject{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "foo/v1",
				Kind:       "Bar",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("object-%d", i),
				Namespace:         fmt.Sprintf("namespace-%d", i%10), // Distribute across 10 namespaces
				UID:               types.UID(fmt.Sprintf("uid-%d", i)),
				ResourceVersion:   fmt.Sprintf("%d", i+1000),
				Generation:        1,
				CreationTimestamp: metav1.NewTime(now.Add(-time.Duration(i) * time.Minute)),
				Labels: map[string]string{
					"app":   "test",
					"idx":   fmt.Sprintf("%d", i),
					"shard": fmt.Sprintf("%d", i%5),
				},
			},
			Spec: map[string]any{
				"field1": fmt.Sprintf("value-%d", i),
				"field2": i * 100,
				"nested": map[string]any{
					"key": "value",
				},
			},
		}
		objects[i] = obj
	}

	return objects
}

// benchmarkKind returns a reusable Kind for benchmarks.
func benchmarkKind() resource.Kind {
	sch := resource.NewSimpleSchema("foo", "v1", &resource.UntypedObject{}, &resource.UntypedList{}, resource.WithKind("Bar"))
	return resource.Kind{
		Schema: sch,
		Codecs: map[resource.KindEncoding]resource.Codec{
			resource.KindEncodingJSON: resource.NewJSONCodec(),
		},
	}
}

// setClientFeature sets the client feature environment variable.
func setClientFeature(feature clientfeatures.Feature, value bool) error {
	if value {
		return os.Setenv(fmt.Sprintf("KUBE_FEATURE_%s", feature), "true")
	}

	return os.Unsetenv(fmt.Sprintf("KUBE_FEATURE_%s", feature))
}
