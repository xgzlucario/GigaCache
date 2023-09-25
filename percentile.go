package cache

import "slices"

const percentileSize = 100 * 10000

// Percentile
type Percentile struct {
	data   []float64
	sorted bool
	pos    int
}

// NewPercentile
func NewPercentile(data ...float64) *Percentile {
	p := &Percentile{
		data: make([]float64, 0, percentileSize),
	}
	for _, d := range data {
		p.Add(d)
	}
	return p
}

// Add
func (p *Percentile) Add(data float64) {
	p.sorted = false
	if len(p.data) == percentileSize {
		p.pos = (p.pos + 1) % percentileSize
		p.data[p.pos] = data

	} else {
		p.data = append(p.data, data)
	}
}

func (p *Percentile) sort() {
	if !p.sorted {
		slices.Sort(p.data)
	}
}

// Percentile
func (p *Percentile) Percentile(percentile float64) float64 {
	p.sort()
	i := (percentile / 100) * float64(len(p.data))
	return p.data[int(i)]
}

// Min
func (p *Percentile) Min() float64 {
	p.sort()
	return p.data[0]
}

// Max
func (p *Percentile) Max() float64 {
	p.sort()
	return p.data[len(p.data)-1]
}

// Avg
func (p *Percentile) Avg() float64 {
	var sum float64
	for _, v := range p.data {
		sum += v
	}
	return sum / float64(len(p.data))
}
