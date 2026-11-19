package llmadmit

import "testing"

func TestMergedExportRequiresSameTarget(t *testing.T) {
	base := Run{Suite: "k3-v1", BaseURL: "https://gw.example.com/v1", Model: "kimi-k3", Channel: "channel-a"}
	if !sameMergeTarget([]Run{base, base}) {
		t.Fatal("same target runs should be mergeable")
	}
	for name, changed := range map[string]Run{
		"channel": {Suite: base.Suite, BaseURL: base.BaseURL, Model: base.Model, Channel: "channel-b"},
		"gateway": {Suite: base.Suite, BaseURL: "https://other.example.com/v1", Model: base.Model, Channel: base.Channel},
		"model":   {Suite: base.Suite, BaseURL: base.BaseURL, Model: "other-model", Channel: base.Channel},
		"suite":   {Suite: "other-suite", BaseURL: base.BaseURL, Model: base.Model, Channel: base.Channel},
	} {
		t.Run(name, func(t *testing.T) {
			if sameMergeTarget([]Run{base, changed}) {
				t.Fatalf("different %s unexpectedly mergeable", name)
			}
		})
	}
}
