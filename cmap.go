package cache

import "github.com/cockroachdb/swiss"

type cmap struct {
	m *swiss.Map[string, Idx]
}

func newCMap() *cmap {
	return &cmap{m: swiss.New[string, Idx](8)}
}

func (m *cmap) Get(k string) (Idx, bool) {
	if m.m.Len() == 0 {
		return 0, false
	}
	return m.m.Get(k)
}

func (m *cmap) Put(k string, v Idx) {
	m.m.Put(k, v)
}

func (m *cmap) Delete(k string) {
	m.m.Delete(k)
}

func (m *cmap) All(f func(string, Idx) bool) {
	if m.m.Len() == 0 {
		return
	}
	m.m.All(f)
}

func (m *cmap) Len() int {
	return m.m.Len()
}
