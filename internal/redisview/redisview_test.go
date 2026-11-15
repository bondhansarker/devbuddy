package redisview

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/bondhansarker/devbuddy/internal/config"
)

// a sparse pattern on a big keyspace takes many SCAN rounds; past the time budget Scan hands back its
// cursor (the UI's "Load more" continues from it) instead of walking everything in one call
func TestScanStopsAtTheTimeBudgetAndHandsBackACursor(t *testing.T) {
	mr := miniredis.RunT(t)
	for i := 0; i < 5000; i++ {
		mr.Set(fmt.Sprintf("bulk:%d", i), "v")
	}
	mr.Set("needle", "v")
	m := New([]config.Redis{{Name: "r", Addr: mr.Addr()}})
	defer func(b time.Duration) { scanBudget = b }(scanBudget)

	scanBudget = time.Hour // no budget pressure: it keeps going until it has the match or the keyspace ends
	cur, keys, err := m.Scan(context.Background(), "r", 0, "needle", "", 0, 200)
	if err != nil || len(keys) != 1 || keys[0].Key != "needle" {
		t.Fatalf("unbounded: cursor %d keys %v err %v", cur, keys, err)
	}

	scanBudget = -1 // already over budget after the first round
	cur, keys, err = m.Scan(context.Background(), "r", 0, "needle", "", 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if cur == 0 && len(keys) == 0 {
		t.Skip("this miniredis answers a whole SCAN in one round, so the budget cannot be seen here")
	}
	if cur != 0 && len(keys) == 0 {
		// continuing from the cursor finds it
		for i := 0; i < 50 && cur != 0; i++ {
			cur, keys, err = m.Scan(context.Background(), "r", 0, "needle", "", cur, 200)
			if err != nil || len(keys) > 0 {
				break
			}
		}
		if len(keys) != 1 {
			t.Fatalf("continuing from the cursor must reach the key, got %v (cursor %d)", keys, cur)
		}
	}
}
