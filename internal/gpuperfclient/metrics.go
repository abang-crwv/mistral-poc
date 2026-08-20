package gpuperfclient

// registry is the ordered set of perf signals gpu_performance_probe gathers.
// Order is stable (sections 1–8 of the HPC-V query pack) so the probe iterates
// deterministically. Each entry renders through the same join template
// (parse.go); adding a signal is a one-line entry here.
var registry = []MetricSpec{
	// 1. GPU Blaze / compute — GFLOPS by node, gpu, precision.
	{ID: "gpu_blaze_gflops_avg", Title: "GPU Blaze GFLOPS (avg)", Agg: "avg", Rule: "gpu:hpc_verification_gpu_blaze_gflops:avg5m", Dims: []string{"node", "gpu", "precision"}},
	{ID: "gpu_blaze_gflops_min", Title: "GPU Blaze GFLOPS (min)", Agg: "min", Rule: "gpu:hpc_verification_gpu_blaze_gflops:min5m", Dims: []string{"node", "gpu", "precision"}},
	{ID: "gpu_blaze_gflops_max", Title: "GPU Blaze GFLOPS (max)", Agg: "max", Rule: "gpu:hpc_verification_gpu_blaze_gflops:max5m", Dims: []string{"node", "gpu", "precision"}},

	// 2. Thermal / throttle context for Blaze.
	{ID: "gpu_blaze_temp_avg", Title: "GPU Blaze temperature (avg)", Agg: "avg", Rule: "gpu:hpc_verification_gpu_blaze_temp:avg5m", Dims: []string{"node", "gpu", "precision"}},
	{ID: "gpu_blaze_tlimit_min", Title: "GPU Blaze TLimit (min)", Agg: "min", Rule: "gpu:hpc_verification_gpu_blaze_tlimit:min5m", Dims: []string{"node", "gpu", "precision"}},
	{ID: "gpu_blaze_tevents_max", Title: "GPU Blaze thermal events (max)", Agg: "max", Rule: "gpu:hpc_verification_gpu_blaze_tevents:max5m", Dims: []string{"node", "gpu", "precision"}},

	// 3. NVBandwidth / memcpy path.
	{ID: "nvbandwidth_avg", Title: "NVBandwidth (avg)", Agg: "avg", Rule: "gpu:hpc_verification_nvbandwidth:avg5m", Dims: []string{"node", "testcase", "src", "dest"}},

	// 3b. NVLink GEMM bandwidth (mnubergemm). No recording rule — the gamble
	// histogram (hpcv_plugin_mnubergemm_*), so these take the _sum/_count mean.
	// min surfaces the weakest link, the useful post-zap regression signal.
	{ID: "nvlink_gemm_bw_avg", Title: "NVLink GEMM bandwidth (avg)", Agg: "avg", Kind: kindHistogramMean, Rule: "hpcv_plugin_mnubergemm_group_value_avg_nvlink_bw_gbps", Dims: []string{"node", "suite"}},
	{ID: "nvlink_gemm_bw_min", Title: "NVLink GEMM bandwidth (min)", Agg: "avg", Kind: kindHistogramMean, Rule: "hpcv_plugin_mnubergemm_group_value_min_nvlink_bw_gbps", Dims: []string{"node", "suite"}},

	// 4. NCCL bandwidth — observed vs expected. No recording rule exists; the
	// data is the gamble OTel histogram (hpcv_plugin_nccl_value_*), so these take
	// the _sum/_count mean. There is no testcase label — suite + nvls_mode carry
	// the test and NVLS-variant identity (e.g. off vs auto).
	{ID: "nccl_bandwidth_observed_avg", Title: "NCCL bandwidth observed (avg)", Agg: "avg", Kind: kindHistogramMean, Rule: "hpcv_plugin_nccl_value_avg_bandwidth_gbps", Dims: []string{"node", "suite", "nvls_mode"}},
	{ID: "nccl_bandwidth_expected_avg", Title: "NCCL bandwidth expected (avg)", Agg: "avg", Kind: kindHistogramMean, Rule: "hpcv_plugin_nccl_value_expected_bandwidth_gbps", Dims: []string{"node", "suite", "nvls_mode"}},

	// 5. Megatron / training-style performance.
	{ID: "megatron_longest_iter_time_max", Title: "Megatron longest iter time (max)", Agg: "max", Rule: "node:hpc_verification_megatron_lm_longest_iter_time:max5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_shortest_iter_time_min", Title: "Megatron shortest iter time (min)", Agg: "min", Rule: "node:hpc_verification_megatron_lm_shortest_iter_time:min5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_job_dur_avg", Title: "Megatron job duration (avg)", Agg: "avg", Rule: "node:hpc_verification_megatron_lm_job_dur:avg5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_job_dur_max", Title: "Megatron job duration (max)", Agg: "max", Rule: "node:hpc_verification_megatron_lm_job_dur:max5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_iter_time_avg", Title: "Megatron iter time (avg)", Agg: "avg", Rule: "node:hpc_verification_megatron_lm_iter_time:avg5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_longest_iter_max", Title: "Megatron longest iterations (max)", Agg: "max", Rule: "node:hpc_verification_megatron_lm_longest_iter:max5m", Dims: []string{"node", "testcase"}},
	{ID: "megatron_shortest_iter_min", Title: "Megatron shortest iterations (min)", Agg: "min", Rule: "node:hpc_verification_megatron_lm_shortest_iter:min5m", Dims: []string{"node", "testcase"}},

	// 6. IB bandwidth by devpair.
	{ID: "ib_bw_avg", Title: "IB bandwidth (avg)", Agg: "avg", Rule: "devpair:hpc_verification_ib_bw:avg5m", Dims: []string{"node", "testcase", "devpair"}},
	{ID: "ib_bw_min", Title: "IB bandwidth (min)", Agg: "min", Rule: "devpair:hpc_verification_ib_bw:min5m", Dims: []string{"node", "testcase", "devpair"}},

	// 7. IB latency by devpair.
	{ID: "ib_lat_avg", Title: "IB latency (avg)", Agg: "avg", Rule: "devpair:hpc_verification_ib_lat:avg5m", Dims: []string{"node", "testcase", "devpair"}},
	{ID: "ib_lat_p99_avg", Title: "IB latency p99 (avg)", Agg: "avg", Rule: "devpair:hpc_verification_ib_lat_p99:avg5m", Dims: []string{"node", "testcase", "devpair"}},
	{ID: "ib_lat_min", Title: "IB latency (min)", Agg: "min", Rule: "devpair:hpc_verification_ib_lat:min5m", Dims: []string{"node", "testcase", "devpair"}},

	// 8. CUDA sync latency by node.
	{ID: "sync_latency_avg", Title: "CUDA sync latency (avg)", Agg: "avg", Rule: "gpu:hpc_verification_sync_latency:avg5m", Dims: []string{"node"}},

	// 9. CPU perf (avg10m window, unlike the 5m families above). threshold is
	// the pass line the average/min are measured against; keep it last.
	{ID: "cpu_perf_avg", Title: "CPU perf (avg)", Agg: "avg", Rule: "node:hpc_verification_cpu_perf_average:avg10m", Dims: []string{"node", "testcase"}},
	{ID: "cpu_perf_max", Title: "CPU perf (max)", Agg: "max", Rule: "node:hpc_verification_cpu_perf_max:avg10m", Dims: []string{"node", "testcase"}},
	{ID: "cpu_perf_min", Title: "CPU perf (min)", Agg: "min", Rule: "node:hpc_verification_cpu_perf_min:avg10m", Dims: []string{"node", "testcase"}},
	{ID: "cpu_perf_threshold", Title: "CPU perf threshold", Agg: "avg", Rule: "node:hpc_verification_cpu_perf_threshold:avg10m", Dims: []string{"node", "testcase"}},
}

// Metrics returns a copy-safe view of the registry in stable order.
func Metrics() []MetricSpec {
	out := make([]MetricSpec, len(registry))
	copy(out, registry)
	return out
}

// specByID looks up a spec by id.
func specByID(id string) (MetricSpec, bool) {
	for _, s := range registry {
		if s.ID == id {
			return s, true
		}
	}
	return MetricSpec{}, false
}
