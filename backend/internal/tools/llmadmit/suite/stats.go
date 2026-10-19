package suite

import (
	"context"
	"math"
	"sort"
	"time"
)

// Percentile 是 nearest-rank 分位数（§2 口径）。空样本返回 0。
// 样本量规则（§1.4-4）由调用方控制：<10 不出 P95/P99，<30 不出 P99。
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	rank := int(p / 100 * float64(len(sorted)))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// CV 是变异系数（Soak TTFT 漂移判据）。
func CV(values []float64) float64 {
	m := Mean(values)
	if m == 0 {
		return 0
	}
	var sq float64
	for _, v := range values {
		sq += (v - m) * (v - m)
	}
	variance := sq / float64(len(values))
	return math.Sqrt(variance) / m
}

// ToFloat64 把 []int64 毫秒序列转 float64。
func ToFloat64(values []int64) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = float64(v)
	}
	return out
}

// CallWithRetry：429/5xx/传输错误最多重试 2 次（退避 1s/3s），
// 返回最后一次结果与重试次数；原始失败不洗白（§8.1）。
func CallWithRetry(ctx context.Context, fn func() CallResult) (CallResult, int) {
	backoff := []time.Duration{time.Second, 3 * time.Second}
	var last CallResult
	for attempt := 0; ; attempt++ {
		last = fn()
		retryable := last.HTTPStatus == 429 || last.HTTPStatus >= 500 || last.ErrKind == "transport"
		if !retryable || attempt >= 2 {
			return last, attempt
		}
		select {
		case <-ctx.Done():
			return last, attempt
		case <-time.After(backoff[attempt]):
		}
	}
}
