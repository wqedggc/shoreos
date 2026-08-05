package fire

import (
	"testing"
	"time"
)

func TestModelFromProfileKeepsTargetIndependentFromCurrentSpending(t *testing.T) {
	profile := map[string]any{
		"incomePost": "30", "houseTypeCur": "rent", "expRent": "5000", "expFood": "3000",
		"houseTypeMin": "rent", "expQRent": "3000", "expQFood": "2000",
		"assetCash": "20", "assetFund": "30", "assetReturn": "5", "childPlan": "none",
	}
	model := ModelFromProfile(profile, 100_000)
	if model.AnnualIncomeCents != 30_000_000 || model.ManualAnnualExpenseCents != 9_600_000 {
		t.Fatalf("unexpected income/expense: %#v", model)
	}
	if model.FireTargetCents != 150_000_000 {
		t.Fatalf("FireTargetCents = %d, want 150000000", model.FireTargetCents)
	}
	if model.InvestableAssetCents != 49_900_000 {
		t.Fatalf("InvestableAssetCents = %d, want 49900000", model.InvestableAssetCents)
	}
}

func TestHigherSpendingDelaysFireWithoutChangingTarget(t *testing.T) {
	asOf := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.Local)
	model := ScenarioModel{AnnualIncomeCents: 20_000_000, InvestableAssetCents: 10_000_000, FireTargetCents: 50_000_000, AnnualReturnRate: 0.04}
	target := Calculate(model, 8_000_000, asOf)
	actual := Calculate(model, 12_000_000, asOf)
	if target.MonthsToFire == nil || actual.MonthsToFire == nil || *actual.MonthsToFire <= *target.MonthsToFire {
		t.Fatalf("higher spending did not delay FIRE: target=%#v actual=%#v", target, actual)
	}
	if target.FireTargetCents != actual.FireTargetCents {
		t.Fatalf("spending changed FIRE target")
	}
}

func TestNoSavingsHasNoExpectedDate(t *testing.T) {
	model := ScenarioModel{AnnualIncomeCents: 10_000, InvestableAssetCents: 0, FireTargetCents: 100_000}
	projection := Calculate(model, 10_000, time.Now())
	if projection.MonthsToFire != nil || projection.ExpectedFireMonth != "" {
		t.Fatalf("unexpected reachable projection: %#v", projection)
	}
}
