package k3

import (
	"testing"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

func TestSuiteGroupsCoverAllCases(t *testing.T) {
	s := Suite()
	seen := map[string]int{}
	for _, c := range s.Cases {
		if c.Group == "" {
			t.Errorf("用例 %s 缺分组", c.ID)
		}
		if c.Level == suite.LevelGate && c.Group != suite.GroupGate {
			t.Errorf("闸门用例 %s 分组=%s，应为 gate", c.ID, c.Group)
		}
		seen[c.ID]++
		if seen[c.ID] > 1 {
			t.Errorf("用例 ID 重复: %s", c.ID)
		}
	}
	for _, preset := range []suite.Group{suite.PresetFormat, suite.PresetCache, suite.PresetLatency} {
		sel := s.Select(preset)
		if len(sel) == 0 {
			t.Errorf("预设 %q 没有用例", preset)
		}
		gates := 0
		for _, c := range sel {
			if c.Group != string(preset) && c.Group != suite.GroupGate {
				t.Errorf("预设 %q 混入分组 %q 的用例 %s", preset, c.Group, c.ID)
			}
			if c.Level == suite.LevelGate {
				gates++
			}
		}
		if gates == 0 {
			t.Errorf("预设 %q 缺闸门用例", preset)
		}
	}
	if got := len(s.Select(suite.PresetAll)); got != len(s.Cases) {
		t.Errorf("全测=%d 用例总数=%d", got, len(s.Cases))
	}
}

func TestSelectIDsExactAndRejectsUnknown(t *testing.T) {
	s := Suite()
	selected, ok := s.SelectIDs([]string{"M-B7"})
	if !ok {
		t.Fatal("M-B7 should exist")
	}
	if len(selected) != 3 || selected[0].ID != "G1" || selected[1].ID != "G2" || selected[2].ID != "M-B7" {
		t.Fatalf("selected=%v, want [G1 G2 M-B7]", selected)
	}
	if _, ok := s.SelectIDs([]string{"NOT-A-CASE"}); ok {
		t.Fatal("unknown id should be rejected")
	}
}
