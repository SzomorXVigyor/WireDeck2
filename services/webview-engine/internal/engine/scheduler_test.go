package engine

import (
	"testing"
	"time"
)

func TestEffectiveInteractiveFloor_ScalesAbove100Registers(t *testing.T) {
	cases := []struct {
		count int
		want  time.Duration
	}{
		{0, 1000 * time.Millisecond},
		{1, 1000 * time.Millisecond},
		{100, 1000 * time.Millisecond},
		{101, 2000 * time.Millisecond},
		{200, 2000 * time.Millisecond},
		{201, 3000 * time.Millisecond},
		{1000, 10000 * time.Millisecond},
		{9000, 60000 * time.Millisecond},
	}
	for _, tc := range cases {
		if got := effectiveInteractiveFloor(tc.count); got != tc.want {
			t.Errorf("effectiveInteractiveFloor(%d) = %v, want %v", tc.count, got, tc.want)
		}
	}
}
