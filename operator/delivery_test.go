package operator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryFromContext(t *testing.T) {
	assert.Equal(t, DeliveryUnknown, DeliveryFromContext(context.Background()),
		"outside a dispatch there is nothing to report")

	for _, delivery := range []Delivery{DeliveryLive, DeliveryRelist, DeliveryUnknown} {
		assert.Equal(t, delivery, DeliveryFromContext(ContextWithDelivery(context.Background(), delivery)))
	}
}

// A value of another type under the same key must not be mistaken for a delivery, since the key is unexported and
// so the only way to collide is deliberate.
func TestDeliveryFromContextIgnoresOtherValues(t *testing.T) {
	ctx := context.WithValue(context.Background(), deliveryContextKey{}, "live")
	assert.Equal(t, DeliveryUnknown, DeliveryFromContext(ctx))
}
