package config

import (
	"math"
	"testing"
	"time"
)

func TestBudgets(t *testing.T) {
	ok, issues := validateBudgets([]Budget{
		{Period: "day", Amount: 20},
		{Period: "month", Amount: 300, ProfileID: "work"},
		{Period: "week", Amount: 5},
		{Period: "day", Amount: 0},
		{Period: "day", Amount: math.NaN()},
	})
	if len(ok) != 2 || len(issues) != 3 {
		t.Errorf("kept %+v, issues %+v", ok, issues)
	}
	now := time.Date(2026, 11, 6, 23, 30, 0, 0, time.Local)
	if got := (Budget{Period: "day"}).Start(now); got != "2026-11-06" {
		t.Errorf("a day starts %s", got)
	}
	if got := (Budget{Period: "month"}).Start(now); got != "2026-11-01" {
		t.Errorf("a month starts %s", got)
	}
}
