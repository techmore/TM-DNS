package systemstats

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type Stats struct {
	CPUPercent       float64 `json:"cpu_percent"`
	ResidentMB       float64 `json:"resident_mb"`
	HeapMB           float64 `json:"heap_mb"`
	Goroutines       int     `json:"goroutines"`
	AppStorageMB     float64 `json:"app_storage_mb"`
	DBSizeMB         float64 `json:"db_size_mb"`
	WALSizeMB        float64 `json:"wal_size_mb"`
	SHMSizeMB        float64 `json:"shm_size_mb"`
	DataDir          string  `json:"data_dir"`
	DiskTotalGB      float64 `json:"disk_total_gb"`
	DiskUsedGB       float64 `json:"disk_used_gb"`
	DiskFreeGB       float64 `json:"disk_free_gb"`
	DiskUsedPercent  float64 `json:"disk_used_percent"`
	SampleWindowSecs float64 `json:"sample_window_secs"`
}

type Sampler struct {
	dbPath      string
	mu          sync.Mutex
	lastAt      time.Time
	lastCPUSecs float64
	lastCPU     float64
}

func NewSampler(dbPath string) *Sampler {
	return &Sampler{dbPath: dbPath}
}

func (s *Sampler) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cpuSecs := processCPUSeconds()
	window := now.Sub(s.lastAt).Seconds()
	cpuPercent := s.lastCPU
	if !s.lastAt.IsZero() && window > 0 {
		cpuPercent = ((cpuSecs - s.lastCPUSecs) / window) * 100
		if cpuPercent < 0 {
			cpuPercent = 0
		}
	}
	s.lastAt = now
	s.lastCPUSecs = cpuSecs
	s.lastCPU = cpuPercent

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rss := residentMB()
	dataDir := filepath.Dir(s.dbPath)
	if dataDir == "." || dataDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dataDir = cwd
		}
	}
	total, free := diskUsage(dataDir)
	used := total - free
	usedPct := 0.0
	if total > 0 {
		usedPct = used / total * 100
	}
	dbSize := fileSizeMB(s.dbPath)
	walSize := fileSizeMB(s.dbPath + "-wal")
	shmSize := fileSizeMB(s.dbPath + "-shm")

	return Stats{
		CPUPercent:       round(cpuPercent),
		ResidentMB:       round(rss),
		HeapMB:           round(bytesToMB(mem.Alloc)),
		Goroutines:       runtime.NumGoroutine(),
		AppStorageMB:     round(dbSize + walSize + shmSize),
		DBSizeMB:         round(dbSize),
		WALSizeMB:        round(walSize),
		SHMSizeMB:        round(shmSize),
		DataDir:          dataDir,
		DiskTotalGB:      round(bytesToGB(total)),
		DiskUsedGB:       round(bytesToGB(used)),
		DiskFreeGB:       round(bytesToGB(free)),
		DiskUsedPercent:  round(usedPct),
		SampleWindowSecs: round(window),
	}
}

func processCPUSeconds() float64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	user := float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1_000_000
	sys := float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1_000_000
	return user + sys
}

func residentMB() float64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	// Darwin reports Maxrss in bytes.
	return float64(usage.Maxrss) / 1024 / 1024
}

func diskUsage(path string) (total float64, free float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	blockSize := float64(stat.Bsize)
	total = float64(stat.Blocks) * blockSize
	free = float64(stat.Bavail) * blockSize
	return total, free
}

func fileSizeMB(path string) float64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return float64(info.Size()) / 1024 / 1024
}

func bytesToMB(v uint64) float64 {
	return float64(v) / 1024 / 1024
}

func bytesToGB(v float64) float64 {
	return v / 1024 / 1024 / 1024
}

func round(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
