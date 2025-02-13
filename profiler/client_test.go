package profiler

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	client, err := NewClient("xxxxxxxxxxxxxxxxxxx", "xxxxxxxxxxxxxxxx", "")
	if err != nil {
		t.Fatal(err)
	}

	r, err := client.GetCPUProfile(t.Context(), "prod-server", "production", "go", 3*time.Hour, 5)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create("cpu.pprof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	io.Copy(f, r)
}
