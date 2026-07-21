package axiosd

import "testing"

func TestParseNvidiaSMI(t *testing.T) {
	gpus := parseNvidiaSMI("0, NVIDIA RTX 3060, 42, 12288, 6144, 57\n1, NVIDIA RTX 3060, 8, 12288, 1024, 44\n")
	if len(gpus) != 2 {
		t.Fatalf("GPU count = %d, want 2", len(gpus))
	}
	if gpus[0].Name != "NVIDIA RTX 3060" || gpus[0].MemoryUsagePercent != 50 {
		t.Fatalf("first GPU = %+v", gpus[0])
	}
	if gpus[1].MemoryUsedBytes != 1024*1024*1024 {
		t.Fatalf("second GPU used bytes = %d, want 1 GiB", gpus[1].MemoryUsedBytes)
	}
}

func TestParseNvidiaSMISkipsMalformedRows(t *testing.T) {
	gpus := parseNvidiaSMI("not,a,valid,row\n0, GPU, 10, 100, 25, 40\n")
	if len(gpus) != 1 || gpus[0].Index != 0 {
		t.Fatalf("GPUs = %+v, want one valid row", gpus)
	}
}
