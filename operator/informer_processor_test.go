package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufferedQueue(t *testing.T) {
	queue := newBufferedQueue(2)
	go queue.run()
	defer queue.stop()

	// insert in queue first to test if it's not blocking
	queue.push(1)
	queue.push(2)
	// more events than initial buffer size to verify that it grows as expected.
	queue.push(3)
	queue.push(4)

	// verify if the order of output is FIFO
	out := queue.events()
	for i := 1; i <= 4; i++ {
		j := <-out
		assert.Equal(t, i, j)
	}
}
