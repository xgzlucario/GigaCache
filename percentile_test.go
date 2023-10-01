package cache

import "testing"

func TestPercentile(t *testing.T) {
	p := NewPercentile()

	for i := 0; i < 100; i++ {
		p.Add(float64(i))
	}

	if p.Min() != 0 {
		t.Fatalf("want 0, got %v", p.Min())
	}
	if p.Max() != 99 {
		t.Fatalf("want 99, got %v", p.Max())
	}
	if p.Avg() != 49.5 {
		t.Fatalf("want 49.5, got %v", p.Avg())
	}
	if p.Percentile(50) != 50 {
		t.Fatalf("want 49, got %v", p.Percentile(50))
	}
	if p.Percentile(99) != 99 {
		t.Fatalf("want 98, got %v", p.Percentile(99))
	}

	p.Print()

	// large
	p = NewPercentile()
	for i := 100 * 10000; i < 300*10000; i++ {
		p.Add(float64(i))
	}

	if p.Min() != 200*10000 {
		t.Fatalf("want 2000000, got %v", p.Min())
	}
	if p.Max() != 300*10000-1 {
		t.Fatalf("want 2999999, got %v", p.Max())
	}
	if p.Avg() != 250*10000-0.5 {
		t.Fatalf("want 2499999.5, got %v", p.Avg())
	}
	if p.Percentile(50) != 2500000 {
		t.Fatalf("want 2500000, got %.0f", p.Percentile(50))
	}
	if p.Percentile(99) != 2990000 {
		t.Fatalf("want 2990000, got %.0f", p.Percentile(99))
	}
}
