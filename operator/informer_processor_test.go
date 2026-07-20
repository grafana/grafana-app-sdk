package operator

import (
	"testing"
	"time"

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

func TestBufferedQueue_Len(t *testing.T) {
	t.Run("empty queue has zero depth", func(t *testing.T) {
		queue := newBufferedQueue(2)
		assert.Equal(t, int64(0), queue.Len())
	})

	t.Run("depth increments on push and decrements on consume", func(t *testing.T) {
		queue := newBufferedQueue(4)
		go queue.run()
		defer queue.stop()

		queue.push("a")
		queue.push("b")
		queue.push("c")

		out := queue.events()

		<-out
		assert.Eventually(t, func() bool { return queue.Len() == 2 }, time.Second, 10*time.Millisecond)

		<-out
		assert.Eventually(t, func() bool { return queue.Len() == 1 }, time.Second, 10*time.Millisecond)

		<-out
		assert.Eventually(t, func() bool { return queue.Len() == 0 }, time.Second, 10*time.Millisecond)
	})

	t.Run("depth returns to zero after full drain", func(t *testing.T) {
		queue := newBufferedQueue(2)
		go queue.run()
		defer queue.stop()

		for i := 0; i < 10; i++ {
			queue.push(i)
		}

		out := queue.events()
		for i := 0; i < 10; i++ {
			<-out
		}

		assert.Eventually(t, func() bool { return queue.Len() == 0 }, time.Second, 10*time.Millisecond)
	})
}
