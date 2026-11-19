package ui

import (
	"strings"
	"testing"
)

func TestCJKBreaks(t *testing.T) {
	h := string(hairSpace)
	cases := map[string]string{
		"plain English stays":   "plain English stays",
		"中文":                    "中" + h + "文",
		"執行 shell 命令":           "執" + h + "行 shell 命" + h + "令",
		"你好，世界。":                "你" + h + "好，" + h + "世" + h + "界。",
		"見「設定」":                 "見" + h + "「設" + h + "定」",
		"`agentflow.yaml`）進行配置": "`agentflow.yaml`）" + h + "進" + h + "行" + h + "配" + h + "置",
	}
	for in, want := range cases {
		if got := cjkBreaks(in); got != want {
			t.Errorf("cjkBreaks(%q) = %q, want %q", in, strings.ReplaceAll(got, h, "|"), strings.ReplaceAll(want, h, "|"))
		}
	}
}
