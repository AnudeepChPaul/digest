package review

import (
	"path/filepath"
	"testing"
)

func TestLoadAndGroup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, FindingsFile), `{"mode":"findings-json","recommendation":"REQUEST_CHANGES","findings":[
{"severity":"high","category":"bug","title":"A","path":"a.ts","line":3,"body":"b"},
{"severity":"critical","category":"security","title":"B","path":"b.ts","line":9,"body":"c","suggestion":"s"},
{"severity":"high","category":"bug","title":"C","path":"c.ts","line":1,"body":"d"},
{"severity":"weird","title":"D"}]}`)
	report, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Recommendation != "REQUEST_CHANGES" || len(report.Findings) != 4 {
		t.Fatalf("report = %+v", report)
	}
	groups := GroupBySeverity(report.Findings)
	if len(groups) != 3 || groups[0].Severity != "critical" || groups[1].Severity != "high" || groups[2].Severity != "weird" {
		t.Fatalf("groups = %+v", groups)
	}
	if len(groups[1].Findings) != 2 || groups[1].Findings[0].Title != "A" {
		t.Errorf("high group = %+v", groups[1])
	}
	flat := Flatten(groups)
	if len(flat) != 4 || flat[0].Title != "B" || flat[3].Title != "D" {
		t.Errorf("flat = %+v", flat)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Errorf("expected error")
	}
}
