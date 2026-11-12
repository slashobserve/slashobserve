package agent

import "testing"

func TestBudgetLimits(t *testing.T) {
	if maxCutOffsInRow != 2 || defaultSessionRenewBytes != 256<<10 {
		t.Fatalf("budgets = %d %d", maxCutOffsInRow, defaultSessionRenewBytes)
	}
}
