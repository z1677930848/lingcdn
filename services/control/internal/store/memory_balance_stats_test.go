package store

import (
	"context"
	"testing"
	"time"
)

func TestMemoryAdminRechargeStats_SplitAndNetByDay(t *testing.T) {
	m := NewMemory("", "")
	ctx := context.Background()

	day1PaidAt := time.Date(2026, 2, 5, 10, 0, 0, 0, time.Local)
	day2PaidAt := time.Date(2026, 2, 6, 11, 0, 0, 0, time.Local)

	m.mu.Lock()
	m.balanceRecharges["r1"] = &BalanceRecharge{ID: "r1", UserID: "u1", AmountCents: 1000, Status: "paid", PaidAt: day1PaidAt, CreatedAt: day1PaidAt}
	m.balanceRecharges["r2"] = &BalanceRecharge{ID: "r2", UserID: "u2", AmountCents: 2000, Status: "paid", PaidAt: day2PaidAt, CreatedAt: day2PaidAt}
	m.balanceRecharges["r3"] = &BalanceRecharge{ID: "r3", UserID: "u3", AmountCents: 5000, Status: "pending", CreatedAt: day1PaidAt}

	m.balanceTransactions["u1"] = []*BalanceTransaction{
		{ID: "t1", UserID: "u1", Type: "adjust", AmountCents: 300, CreatedAt: time.Date(2026, 2, 5, 12, 0, 0, 0, time.Local)},
		{ID: "t2", UserID: "u1", Type: "adjust", AmountCents: -100, CreatedAt: time.Date(2026, 2, 5, 13, 0, 0, 0, time.Local)},
		{ID: "t3", UserID: "u1", Type: "adjust", AmountCents: -50, CreatedAt: time.Date(2026, 2, 6, 9, 0, 0, 0, time.Local)},
		{ID: "t4", UserID: "u1", Type: "consume", AmountCents: -999, CreatedAt: time.Date(2026, 2, 6, 10, 0, 0, 0, time.Local)},
	}
	m.mu.Unlock()

	from := time.Date(2026, 2, 5, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 2, 6, 0, 0, 0, 0, time.Local)
	stats, err := m.AdminRechargeStats(ctx, from, to)
	if err != nil {
		t.Fatalf("AdminRechargeStats error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 days, got %d", len(stats))
	}

	day1 := stats[0]
	if day1.Day != "2026-02-05" {
		t.Fatalf("expected day1 2026-02-05, got %s", day1.Day)
	}
	if day1.RechargeCents != 1000 || day1.RechargeCount != 1 {
		t.Fatalf("day1 recharge mismatch: cents=%d count=%d", day1.RechargeCents, day1.RechargeCount)
	}
	if day1.AdjustCents != 200 || day1.AdjustCount != 2 {
		t.Fatalf("day1 adjust mismatch: cents=%d count=%d", day1.AdjustCents, day1.AdjustCount)
	}
	if day1.TotalCents != 1200 || day1.TotalCount != 3 {
		t.Fatalf("day1 total mismatch: cents=%d count=%d", day1.TotalCents, day1.TotalCount)
	}

	day2 := stats[1]
	if day2.Day != "2026-02-06" {
		t.Fatalf("expected day2 2026-02-06, got %s", day2.Day)
	}
	if day2.RechargeCents != 2000 || day2.RechargeCount != 1 {
		t.Fatalf("day2 recharge mismatch: cents=%d count=%d", day2.RechargeCents, day2.RechargeCount)
	}
	if day2.AdjustCents != -50 || day2.AdjustCount != 1 {
		t.Fatalf("day2 adjust mismatch: cents=%d count=%d", day2.AdjustCents, day2.AdjustCount)
	}
	if day2.TotalCents != 1950 || day2.TotalCount != 2 {
		t.Fatalf("day2 total mismatch: cents=%d count=%d", day2.TotalCents, day2.TotalCount)
	}
}

func TestMemoryAdminRechargeStats_OnlyAdjustAndOnlyRecharge(t *testing.T) {
	m := NewMemory("", "")
	ctx := context.Background()

	m.mu.Lock()
	m.balanceTransactions["u1"] = []*BalanceTransaction{
		{ID: "a1", UserID: "u1", Type: "adjust", AmountCents: 888, CreatedAt: time.Date(2026, 2, 7, 8, 0, 0, 0, time.Local)},
	}
	m.balanceRecharges["r1"] = &BalanceRecharge{ID: "r1", UserID: "u2", AmountCents: 666, Status: "paid", PaidAt: time.Date(2026, 2, 8, 9, 0, 0, 0, time.Local), CreatedAt: time.Date(2026, 2, 8, 9, 0, 0, 0, time.Local)}
	m.mu.Unlock()

	from := time.Date(2026, 2, 7, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 2, 8, 0, 0, 0, 0, time.Local)
	stats, err := m.AdminRechargeStats(ctx, from, to)
	if err != nil {
		t.Fatalf("AdminRechargeStats error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 days, got %d", len(stats))
	}

	if stats[0].Day != "2026-02-07" || stats[0].RechargeCents != 0 || stats[0].AdjustCents != 888 {
		t.Fatalf("unexpected 2026-02-07 stats: %+v", stats[0])
	}
	if stats[1].Day != "2026-02-08" || stats[1].RechargeCents != 666 || stats[1].AdjustCents != 0 {
		t.Fatalf("unexpected 2026-02-08 stats: %+v", stats[1])
	}
}
