package mysql

import "testing"

func TestClassificationRuleNeedsReview(t *testing.T) {
	complete := ClassificationRule{
		CategoryGroup:  "交通出行",
		CategoryDetail: "公交地铁",
		Purpose:        "生存与责任",
		Necessity:      "必需",
		Planning:       "计划内",
		Recurrence:     "不规律重复",
	}
	if classificationRuleNeedsReview(complete) {
		t.Fatal("a complete user rule must clear the review state")
	}

	incomplete := complete
	incomplete.Planning = "未知"
	if !classificationRuleNeedsReview(incomplete) {
		t.Fatal("a rule with an unknown dimension must remain reviewable")
	}
}

func TestRuleConditionSupportsExactSourceCategory(t *testing.T) {
	condition, value, err := ruleCondition("source_category", "服饰装扮")
	if err != nil {
		t.Fatal(err)
	}
	if condition != "t.source_category = ?" || value != "服饰装扮" {
		t.Fatalf("unexpected source category condition: %q %#v", condition, value)
	}
}
