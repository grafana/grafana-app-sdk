package operator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/puzpuzpuz/xsync/v2"
	"github.com/stretchr/testify/assert"
)

func TestListMap_AddItem(t *testing.T) {
	// Ensure that concurrent adds don't get lost
	t.Run("concurrent adds, same key", func(t *testing.T) {
		m := NewListMap[string]()
		c := make(chan []string, 0)
		wg := sync.WaitGroup{}
		key := "foo"
		perGoroutine := 1000

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				finalList := make([]string, perGoroutine)
				for i := 0; i < perGoroutine; i++ {
					val := fmt.Sprintf("%d-%d", idx, i)
					finalList[i] = val
					m.AddItem(key, val)
				}
				c <- finalList
				wg.Done()
			}(i)
		}

		go func() {
			wg.Wait()
			close(c)
		}()

		finalList := make([]string, 0)
		for l := range c {
			finalList = append(finalList, l...)
		}

		inMap := make([]string, 0)
		m.Range(key, func(index int, value string) {
			inMap = append(inMap, value)
		})

		assert.ElementsMatch(t, finalList, inMap)
	})

	t.Run("concurrent adds, different keys", func(t *testing.T) {
		m := NewListMap[string]()
		final := xsync.NewMapOf[[]string]()
		wg := sync.WaitGroup{}
		perGoroutine := 1000

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				finalList := make([]string, perGoroutine)
				key := fmt.Sprintf("k%d", idx)
				for i := 0; i < perGoroutine; i++ {
					val := fmt.Sprintf("%d-%d", idx, i)
					finalList[i] = val
					m.AddItem(key, val)
				}
				final.Store(key, finalList)
				wg.Done()
			}(i)
		}

		wg.Wait()

		assert.Equal(t, final.Size(), m.Size())
		final.Range(func(key string, value []string) bool {
			inMap := make([]string, 0)
			m.Range(key, func(index int, value string) {
				inMap = append(inMap, value)
			})
			assert.ElementsMatch(t, value, inMap)
			return true
		})
	})
}

func TestListMap_ItemAt(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		m := NewListMap[string]()
		v, ok := m.ItemAt("foo", 0)
		assert.False(t, ok)
		assert.Equal(t, "", v)
	})
	t.Run("index too high", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		v, ok := m.ItemAt("foo", 1)
		assert.False(t, ok)
		assert.Equal(t, "", v)
	})
	t.Run("success", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar1")
		m.AddItem("foo", "bar2")
		v, ok := m.ItemAt("foo", 1)
		assert.True(t, ok)
		assert.Equal(t, "bar2", v)
	})
}

func TestListMap_Range(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz")
		anyItem := false
		m.Range("bar", func(index int, value string) {
			anyItem = true
		})
		assert.False(t, anyItem)
	})

	t.Run("empty list", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItemAt("foo", 0)
		anyItem := false
		m.Range("bar", func(index int, value string) {
			anyItem = true
		})
		assert.False(t, anyItem)
	})

	t.Run("success", func(t *testing.T) {
		m := NewListMap[string]()
		desired := []string{"bar", "baz"}
		m.AddItem("foo", desired...)
		found := make([]string, 0)
		m.Range("foo", func(index int, value string) {
			assert.Equal(t, desired[index], value)
			found = append(found, value)
		})
		assert.Equal(t, len(desired), len(found))
	})
}

func TestListMap_RangeAll(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m := NewListMap[string]()
		anyItem := false
		m.RangeAll(func(key string, index int, value string) {
			anyItem = true
		})
		assert.False(t, anyItem)
	})

	t.Run("only empty lists", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItemAt("foo", 0)
		m.AddItem("bar", "foo")
		m.RemoveItemAt("bar", 0)
		anyItem := false
		m.RangeAll(func(key string, index int, value string) {
			anyItem = true
		})
		assert.False(t, anyItem)
	})

	t.Run("success", func(t *testing.T) {
		desired := map[string][]string{
			"foo": {"bar", "baz"},
			"bar": {"foobar", "foobaz"},
		}
		m := NewListMap[string]()
		m.AddItem("foo", desired["foo"]...)
		m.AddItem("bar", desired["bar"]...)
		found := make(map[string][]string)
		m.RangeAll(func(key string, index int, value string) {
			assert.Equal(t, desired[key][index], value)
			s, ok := found[key]
			if !ok {
				s = make([]string, 0)
			}
			found[key] = append(s, value)
		})
		assert.Equal(t, len(desired), len(found))
		for k, v := range desired {
			assert.Equal(t, len(v), len(found[k]))
		}
	})
}

func TestListMap_RemoveKey(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveKey("bar")
		assert.Equal(t, 1, m.Size())
	})

	t.Run("success", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveKey("foo")
		assert.Equal(t, 0, m.Size())
	})
}

func TestListMap_RemoveItem(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItem("baz", func(s string) bool {
			return true
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
	})

	t.Run("no match", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItem("foo", func(s string) bool {
			return false
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
	})

	t.Run("first item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz")
		m.RemoveItem("foo", func(s string) bool {
			return s == "bar"
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "baz", elem)
	})

	t.Run("last item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz")
		m.RemoveItem("foo", func(s string) bool {
			return s == "baz"
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "bar", elem)
	})

	t.Run("only item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItem("foo", func(s string) bool {
			return true
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 0, m.KeySize("foo"))
	})

	t.Run("middle item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz", "foobar")
		m.RemoveItem("foo", func(s string) bool {
			return s == "baz"
		})
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 2, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "bar", elem)
		elem, ok = m.ItemAt("foo", 1)
		assert.True(t, ok)
		assert.Equal(t, "foobar", elem)
	})
}

func TestListMap_RemoveItemAt(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItemAt("baz", 0)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
	})

	t.Run("index too high", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItemAt("foo", 1)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
	})

	t.Run("first index", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz")
		m.RemoveItemAt("foo", 0)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "baz", elem)
	})

	t.Run("last index", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz")
		m.RemoveItemAt("foo", 1)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 1, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "bar", elem)
	})

	t.Run("only item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar")
		m.RemoveItemAt("foo", 0)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 0, m.KeySize("foo"))
	})

	t.Run("middle item", func(t *testing.T) {
		m := NewListMap[string]()
		m.AddItem("foo", "bar", "baz", "foobar")
		m.RemoveItemAt("foo", 1)
		assert.Equal(t, 1, m.Size())
		assert.Equal(t, 2, m.KeySize("foo"))
		elem, ok := m.ItemAt("foo", 0)
		assert.True(t, ok)
		assert.Equal(t, "bar", elem)
		elem, ok = m.ItemAt("foo", 1)
		assert.True(t, ok)
		assert.Equal(t, "foobar", elem)
	})
}

func TestListMap_Size(t *testing.T) {
	tests := []struct {
		elems        []tuple[string, string]
		expectedSize int
	}{
		{
			elems: []tuple[string, string]{
				{"k1", "v1"},
				{"k1", "v2"},
				{"k2", "v1"},
			},
			expectedSize: 2,
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			m := NewListMap[string]()
			for _, tpl := range test.elems {
				m.AddItem(tpl.first, tpl.second)
			}
			assert.Equal(t, test.expectedSize, m.Size())
		})
	}
}

func TestListMap_KeySize(t *testing.T) {
	tests := []struct {
		elems []tuple[string, string]
		sizes map[string]int
	}{
		{
			elems: []tuple[string, string]{},
			sizes: map[string]int{
				"k1": 0,
			},
		},
		{
			elems: []tuple[string, string]{
				{"k1", "v1"},
				{"k1", "v2"},
				{"k2", "v1"},
			},
			sizes: map[string]int{
				"k1": 2,
				"k2": 1,
			},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			m := NewListMap[string]()
			for _, tpl := range test.elems {
				m.AddItem(tpl.first, tpl.second)
			}
			for k, v := range test.sizes {
				assert.Equal(t, v, m.KeySize(k))
			}
		})
	}
}

func TestListMap_Keys(t *testing.T) {
	tests := []struct {
		elems []tuple[string, string]
		keys  []string
	}{
		{
			elems: []tuple[string, string]{},
			keys:  []string{},
		},
		{
			elems: []tuple[string, string]{
				{"k1", "v1"},
				{"k1", "v2"},
				{"k2", "v1"},
			},
			keys: []string{"k1", "k2"},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			m := NewListMap[string]()
			for _, tpl := range test.elems {
				m.AddItem(tpl.first, tpl.second)
			}
			assert.ElementsMatch(t, test.keys, m.Keys())
		})
	}
}

type tuple[T1, T2 any] struct {
	first  T1
	second T2
}
