package fire

import (
	"math"
	"strconv"
	"time"
)

const maxProjectionMonths = 1200

type ScenarioModel struct {
	AnnualIncomeCents         int64   `json:"annualIncomeCents"`
	ManualAnnualExpenseCents  int64   `json:"manualAnnualExpenseCents"`
	MinimumAnnualExpenseCents int64   `json:"minimumAnnualExpenseCents"`
	TotalAssetCents           int64   `json:"totalAssetCents"`
	ReservedFundCents         int64   `json:"reservedFundCents"`
	MortgagePrincipalCents    int64   `json:"mortgagePrincipalCents"`
	InvestableAssetCents      int64   `json:"investableAssetCents"`
	FireTargetCents           int64   `json:"fireTargetCents"`
	AnnualReturnRate          float64 `json:"annualReturnRate"`
}

type Projection struct {
	AnnualExpenseCents   int64  `json:"annualExpenseCents"`
	AnnualSavingsCents   int64  `json:"annualSavingsCents"`
	FireTargetCents      int64  `json:"fireTargetCents"`
	InvestableAssetCents int64  `json:"investableAssetCents"`
	GapCents             int64  `json:"gapCents"`
	MonthsToFire         *int   `json:"monthsToFire"`
	ExpectedFireMonth    string `json:"expectedFireMonth,omitempty"`
}

func ModelFromProfile(profile map[string]any, reservedFundCents int64) ScenarioModel {
	currentMonthly := houseCost(profile, "cur") + yuan(profile, "expFood") + yuan(profile, "expTransport") +
		yuan(profile, "expPet") + yuan(profile, "expEntertain") + yuan(profile, "expInsurance") + yuan(profile, "expOther")
	minimumMortgageMonthly := mortgageMonthly(profile, "min")
	// Quit expense fields fall back to current when unset
	minimumMonthly := houseCost(profile, "min") + yuanQ(profile, "expQFood", "expFood") + yuanQ(profile, "expQTransport", "expTransport") +
		yuanQ(profile, "expQPet", "expPet") + yuanQ(profile, "expQEntertain", "expEntertain") + yuanQ(profile, "expQInsurance", "expInsurance") + yuanQ(profile, "expQOther", "expOther") +
		yuan(profile, "pensionSelfPay") + yuan(profile, "medicalSelfPay")
	currentAnnual := currentMonthly * 12
	minimumAnnual := minimumMonthly * 12
	if text(profile, "childPlan") == "one" {
		childAnnual := wanYuan(profile, "childCost")
		currentAnnual += childAnnual
		minimumAnnual += childAnnual
	}
	mortgageYears := number(profile, "mortgageYearsLeftQ")
	mortgagePrincipal := int64(math.Round(float64(minimumMortgageMonthly) * 12 * mortgageYears * 0.85))
	totalAssets := wanYuan(profile, "assetCash") + wanYuan(profile, "assetDeposit") + wanYuan(profile, "assetFund") +
		wanYuan(profile, "assetStock") + wanYuan(profile, "assetPension")
	investable := max64(totalAssets-mortgagePrincipal-reservedFundCents, 0)
	longTermAnnual := max64(minimumAnnual-minimumMortgageMonthly*12, 0)
	return ScenarioModel{
		AnnualIncomeCents: wanYuan(profile, "incomePost"), ManualAnnualExpenseCents: currentAnnual,
		MinimumAnnualExpenseCents: minimumAnnual, TotalAssetCents: totalAssets, ReservedFundCents: reservedFundCents,
		MortgagePrincipalCents: mortgagePrincipal, InvestableAssetCents: investable,
		FireTargetCents:  longTermAnnual*25 + mortgagePrincipal,
		AnnualReturnRate: number(profile, "assetReturn") / 100,
	}
}

func Calculate(model ScenarioModel, annualExpenseCents int64, asOf time.Time) Projection {
	annualSavings := max64(model.AnnualIncomeCents-annualExpenseCents, 0)
	gap := max64(model.FireTargetCents-model.InvestableAssetCents, 0)
	result := Projection{
		AnnualExpenseCents: annualExpenseCents, AnnualSavingsCents: annualSavings,
		FireTargetCents: model.FireTargetCents, InvestableAssetCents: model.InvestableAssetCents, GapCents: gap,
	}
	if gap == 0 {
		months := 0
		result.MonthsToFire = &months
		result.ExpectedFireMonth = asOf.Format("2006-01")
		return result
	}
	if annualSavings <= 0 {
		return result
	}
	assets := float64(model.InvestableAssetCents)
	monthlySavings := float64(annualSavings) / 12
	monthlyReturn := model.AnnualReturnRate / 12
	for month := 1; month <= maxProjectionMonths; month++ {
		assets = assets*(1+monthlyReturn) + monthlySavings
		if assets >= float64(model.FireTargetCents) {
			result.MonthsToFire = intPointer(month)
			result.ExpectedFireMonth = asOf.AddDate(0, month, 0).Format("2006-01")
			break
		}
	}
	return result
}

func CalculateWithAdditionalAssets(model ScenarioModel, annualExpenseCents, additionalAssetCents int64, asOf time.Time) Projection {
	model.InvestableAssetCents += additionalAssetCents
	return Calculate(model, annualExpenseCents, asOf)
}

func houseCost(profile map[string]any, section string) int64 {
	houseType := text(profile, "houseTypeCur")
	if section == "min" {
		houseType = text(profile, "houseTypeMin")
	}
	switch {
	case section == "cur" && houseType == "rent":
		return yuan(profile, "expRent")
	case section == "cur" && houseType == "mortgage":
		return yuan(profile, "expMortgage") + yuan(profile, "expProperty")
	case section == "cur":
		return yuan(profile, "expProperty2")
	case houseType == "rent":
		return yuanQ(profile, "expQRent", "expRent")
	case houseType == "mortgage":
		return yuanQ(profile, "expQMortgage", "expMortgage") + yuanQ(profile, "expQProperty", "expProperty")
	default:
		return yuanQ(profile, "expQProperty2", "expProperty2")
	}
}

func mortgageMonthly(profile map[string]any, section string) int64 {
	if section == "min" && text(profile, "houseTypeMin") == "mortgage" {
		return yuan(profile, "expQMortgage")
	}
	if section == "cur" && text(profile, "houseTypeCur") == "mortgage" {
		return yuan(profile, "expMortgage")
	}
	return 0
}

func yuan(profile map[string]any, key string) int64 {
	return int64(math.Round(number(profile, key) * 100))
}

// yuanQ returns the value of qKey, falling back to cKey when qKey is zero or missing.
func yuanQ(profile map[string]any, qKey, cKey string) int64 {
	v := number(profile, qKey)
	if v == 0 {
		v = number(profile, cKey)
	}
	return int64(math.Round(v * 100))
}

func wanYuan(profile map[string]any, key string) int64 {
	return int64(math.Round(number(profile, key) * 10_000 * 100))
}

func number(profile map[string]any, key string) float64 {
	value := profile[key]
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case jsonNumber:
		parsed, _ := strconv.ParseFloat(string(typed), 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}

type jsonNumber string

func text(profile map[string]any, key string) string {
	value, _ := profile[key].(string)
	return value
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func intPointer(value int) *int {
	return &value
}
