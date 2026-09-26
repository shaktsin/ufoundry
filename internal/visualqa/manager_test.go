package visualqa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVisualQABlackBoxSession(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("system-browser integration coverage currently runs on macOS")
	}
	if os.Getenv("UMCODE_CHROMIUM_PATH") == "" {
		chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chrome); err != nil {
			t.Skip("Chrome is not installed")
		}
		t.Setenv("UMCODE_CHROMIUM_PATH", chrome)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><title>Visual QA fixture</title><body><canvas id="noise" width="640" height="420"></canvas><button onclick="document.querySelector('main').textContent='changed'">Change</button><main>initial</main><img src="/missing.png"><script>const c=document.querySelector('#noise').getContext('2d'),d=c.createImageData(640,420);let x=123456789;for(let i=0;i<d.data.length;i+=4){x=(x*1664525+1013904223)>>>0;d.data[i]=x>>>24;x=(x*1664525+1013904223)>>>0;d.data[i+1]=x>>>24;x=(x*1664525+1013904223)>>>0;d.data[i+2]=x>>>24;d.data[i+3]=255}c.putImageData(d,0,0)</script></body></html>`))
	}))
	defer server.Close()

	m := NewManager(context.Background())
	defer m.Close()
	root := t.TempDir()
	report, err := m.Start("thread-1", root, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(report.Artifacts) != 1 || !strings.Contains(report.Snapshot, "Visual QA fixture") || !strings.Contains(report.Snapshot, "brokenImages") {
		t.Fatalf("unexpected initial report: %+v", report)
	}
	if report.Artifacts[0].Bytes <= 32<<10 {
		t.Fatalf("screenshot did not exceed the previous websocket limit: %d bytes", report.Artifacts[0].Bytes)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(report.Artifacts[0].Path))); err != nil {
		t.Fatalf("screenshot artifact was not saved: %v", err)
	}
	session := m.ForThread("thread-1")
	if session == nil {
		t.Fatal("session was not retained")
	}
	snapshot, err := session.Act(context.Background(), "click", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, "changed") {
		t.Fatalf("click did not change rendered page: %s", snapshot)
	}
}
